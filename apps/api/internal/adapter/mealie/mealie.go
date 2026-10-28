// Package mealie is the driven adapter for Mealie: recipes and the shopping list. It talks to
// Mealie through its MCP server, which exposes one tool per Mealie endpoint and holds Mealie's
// own credentials; Homeloom only has the MCP server's bearer token.
package mealie

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// maxRecipes bounds how many recipes one listing reads.
const maxRecipes = 1000

type Client struct {
	endpoint string
	http     *http.Client
	mcp      *mcp.Client
	web      *url.URL // Mealie itself, for links; nil: no links
	listID   string   // empty: the most recently used shopping list

	mu        sync.Mutex
	session   *mcp.ClientSession // reused; dropped after a transport error
	groupSlug string             // for links into Mealie; looked up once
}

var _ port.RecipeBook = (*Client)(nil)

// New talks to the Mealie MCP server at mcpURL with its bearer token. webURL is Mealie's own
// address, used only for "open in Mealie" links, and may be empty. listID picks the shopping list;
// if empty, ingredients go to the list that changed last.
func New(mcpURL, token, webURL, listID string) (*Client, error) {
	u, err := url.Parse(mcpURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("MEALIE_MCP_URL %q is not an http(s) URL", mcpURL)
	}
	c := &Client{
		endpoint: mcpURL,
		http:     &http.Client{Timeout: 8 * time.Second, Transport: bearer{token: token, next: http.DefaultTransport}},
		mcp:      mcp.NewClient(&mcp.Implementation{Name: "homeloom", Version: "1"}, nil),
		listID:   listID,
	}
	if webURL != "" {
		w, err := url.Parse(strings.TrimRight(webURL, "/"))
		if err != nil || (w.Scheme != "http" && w.Scheme != "https") || w.Host == "" {
			return nil, fmt.Errorf("MEALIE_URL %q is not an http(s) URL", webURL)
		}
		c.web = w
	}
	return c, nil
}

type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

type tag struct {
	Name string `json:"name"`
}

type summary struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	TotalTime   string `json:"totalTime"`
	Categories  []tag  `json:"recipeCategory"`
	Tags        []tag  `json:"tags"`
}

func (s summary) domain() domain.RecipeSummary {
	tags := make([]string, 0, len(s.Categories)+len(s.Tags))
	for _, t := range append(s.Categories, s.Tags...) {
		tags = append(tags, t.Name)
	}
	return domain.RecipeSummary{Slug: s.Slug, Name: s.Name, Description: s.Description, TotalTime: s.TotalTime, Tags: tags}
}

func (c *Client) Recipes(ctx context.Context) ([]domain.RecipeSummary, error) {
	var page struct {
		Items []summary `json:"items"`
	}
	args := map[string]any{"page": 1, "perPage": maxRecipes, "orderBy": "name", "orderDirection": "asc"}
	if err := c.call(ctx, "list_recipes", args, &page); err != nil {
		return nil, err
	}
	out := make([]domain.RecipeSummary, len(page.Items))
	for i, s := range page.Items {
		out[i] = s.domain()
	}
	return out, nil
}

type recipe struct {
	summary
	ID           string            `json:"id"`
	Yield        string            `json:"recipeYield"`
	Servings     float64           `json:"recipeServings"`
	Ingredients  []json.RawMessage `json:"recipeIngredient"`
	Instructions []struct {
		Text string `json:"text"`
	} `json:"recipeInstructions"`
}

type ingredient struct {
	ReferenceID  string `json:"referenceId"`
	Display      string `json:"display"`
	OriginalText string `json:"originalText"`
	Note         string `json:"note"`
}

func (c *Client) get(ctx context.Context, slug string) (recipe, error) {
	var r recipe
	err := c.call(ctx, "get_recipes_by_slug", map[string]any{"slug": slug}, &r)
	return r, err
}

func (c *Client) Recipe(ctx context.Context, slug string) (domain.Recipe, error) {
	r, err := c.get(ctx, slug)
	if err != nil {
		return domain.Recipe{}, err
	}
	out := domain.Recipe{RecipeSummary: r.domain(), Servings: r.Yield, Ingredients: []domain.Ingredient{}, Steps: []string{}}
	if out.Servings == "" && r.Servings > 0 {
		out.Servings = strconv.FormatFloat(r.Servings, 'f', -1, 64)
	}
	for _, raw := range r.Ingredients {
		var in ingredient
		if json.Unmarshal(raw, &in) != nil || in.ReferenceID == "" {
			continue
		}
		text := firstNonEmpty(in.Display, in.OriginalText, in.Note)
		if text == "" {
			continue
		}
		out.Ingredients = append(out.Ingredients, domain.Ingredient{ID: in.ReferenceID, Text: text})
	}
	for _, step := range r.Instructions {
		if t := strings.TrimSpace(step.Text); t != "" {
			out.Steps = append(out.Steps, t)
		}
	}
	if group := c.group(ctx); group != "" && c.web != nil {
		out.URL = c.web.JoinPath("g", group, "r", r.Slug).String()
	}
	return out, nil
}

func (c *Client) AddToShoppingList(ctx context.Context, slug string, ingredientIDs []string) (domain.ShoppingListAdded, error) {
	r, err := c.get(ctx, slug)
	if err != nil {
		return domain.ShoppingListAdded{}, err
	}
	want := make(map[string]bool, len(ingredientIDs))
	for _, id := range ingredientIDs {
		want[id] = true
	}
	// Mealie wants the ingredients as it stores them, so they are passed on unchanged.
	selected := []json.RawMessage{}
	for _, raw := range r.Ingredients {
		var in ingredient
		if json.Unmarshal(raw, &in) == nil && want[in.ReferenceID] {
			selected = append(selected, raw)
		}
	}
	if len(selected) == 0 {
		return domain.ShoppingListAdded{}, fmt.Errorf("%w: none of the ingredients belong to %s", domain.ErrInvalid, slug)
	}
	id, name, err := c.list(ctx)
	if err != nil {
		return domain.ShoppingListAdded{}, err
	}
	body := []map[string]any{{"recipeId": r.ID, "recipeIncrementQuantity": 1, "recipeIngredients": selected}}
	if err := c.call(ctx, "create_households_shopping_lists_by_item_recipe", map[string]any{"item_id": id, "body": body}, nil); err != nil {
		return domain.ShoppingListAdded{}, err
	}
	return domain.ShoppingListAdded{Added: len(selected), List: name}, nil
}

// list returns the configured shopping list, or else the one that changed last.
func (c *Client) list(ctx context.Context) (id, name string, err error) {
	type list struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if c.listID != "" {
		var l list
		if err := c.call(ctx, "get_households_shopping_lists_by_item", map[string]any{"item_id": c.listID}, &l); err != nil {
			return "", "", err
		}
		return l.ID, l.Name, nil
	}
	var page struct {
		Items []list `json:"items"`
	}
	args := map[string]any{"page": 1, "perPage": 1, "orderBy": "updatedAt", "orderDirection": "desc"}
	if err := c.call(ctx, "list_households_shopping_lists", args, &page); err != nil {
		return "", "", err
	}
	if len(page.Items) == 0 {
		return "", "", fmt.Errorf("%w: Mealie has no shopping list", domain.ErrNotFound)
	}
	return page.Items[0].ID, page.Items[0].Name, nil
}

// group returns the group's slug for recipe links, or "" without MEALIE_URL or if Mealie won't say.
func (c *Client) group(ctx context.Context) string {
	if c.web == nil {
		return ""
	}
	c.mu.Lock()
	slug := c.groupSlug
	c.mu.Unlock()
	if slug == "" {
		var g struct {
			Slug string `json:"slug"`
		}
		if c.call(ctx, "list_groups_self", nil, &g) == nil {
			slug = g.Slug
			c.mu.Lock()
			c.groupSlug = slug
			c.mu.Unlock()
		}
	}
	return slug
}

// httpStatus finds the status of the Mealie call in a tool error, e.g. "HTTP error 404: Not Found".
var httpStatus = regexp.MustCompile(`HTTP error (\d{3})`)

// call runs one MCP tool and decodes its JSON result into out. The MCP server or Mealie being
// down, slow or refusing a token is domain.ErrUnavailable; a 404 from Mealie is domain.ErrNotFound.
func (c *Client) call(ctx context.Context, tool string, args map[string]any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	res, err := c.callTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return fmt.Errorf("%w: mealie mcp %s: %w", domain.ErrUnavailable, tool, err)
	}
	text := ""
	for _, ct := range res.Content {
		if t, ok := ct.(*mcp.TextContent); ok {
			text += t.Text
		}
	}
	if res.IsError {
		status := 0
		if m := httpStatus.FindStringSubmatch(text); m != nil {
			status, _ = strconv.Atoi(m[1])
		}
		switch {
		case status == http.StatusNotFound:
			return fmt.Errorf("%w: mealie %s", domain.ErrNotFound, tool)
		case status == 0 || status == http.StatusUnauthorized || status == http.StatusForbidden || status >= 500:
			return fmt.Errorf("%w: mealie %s: %.200s", domain.ErrUnavailable, tool, text)
		default:
			return fmt.Errorf("mealie %s: %.200s", tool, text)
		}
	}
	if out == nil {
		return nil
	}
	raw := []byte(text)
	if res.StructuredContent != nil {
		if raw, err = json.Marshal(res.StructuredContent); err != nil {
			return err
		}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: mealie %s: %w", domain.ErrUnavailable, tool, err)
	}
	return nil
}

// callTool calls on the open session. A session the server has since forgotten (say, after a
// restart) fails once and is replaced by a fresh one.
func (c *Client) callTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	for attempt := 0; ; attempt++ {
		cs, fresh, err := c.connect(ctx)
		if err != nil {
			return nil, err
		}
		res, err := cs.CallTool(ctx, params)
		if err == nil {
			return res, nil
		}
		c.drop(cs)
		if fresh || attempt > 0 || ctx.Err() != nil {
			return nil, err
		}
	}
}

// connect returns the open MCP session, starting one if needed; fresh tells which.
func (c *Client) connect(ctx context.Context) (*mcp.ClientSession, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return c.session, false, nil
	}
	t := &mcp.StreamableClientTransport{Endpoint: c.endpoint, HTTPClient: c.http, MaxRetries: -1, DisableStandaloneSSE: true}
	cs, err := c.mcp.Connect(ctx, t, nil)
	if err != nil {
		return nil, true, err
	}
	c.session = cs
	return cs, true, nil
}

// drop closes a session that failed, so the next call starts a fresh one.
func (c *Client) drop(cs *mcp.ClientSession) {
	c.mu.Lock()
	if c.session == cs {
		c.session = nil
	}
	c.mu.Unlock()
	_ = cs.Close()
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
