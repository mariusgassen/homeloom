// Package mealie is the driven adapter for Mealie's REST API: recipes and the shopping list.
package mealie

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// maxRecipes bounds how many recipes one listing reads.
const maxRecipes = 1000

type Client struct {
	base   *url.URL
	token  string
	listID string // empty: the most recently used shopping list
	http   *http.Client

	mu        sync.Mutex
	groupSlug string // for links into Mealie; looked up once
}

var _ port.RecipeBook = (*Client)(nil)

// New talks to the Mealie at baseURL with an API token. listID picks the shopping list; if empty,
// ingredients go to the list that changed last.
func New(baseURL, token, listID string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("MEALIE_URL %q is not an http(s) URL", baseURL)
	}
	return &Client{base: u, token: token, listID: listID, http: &http.Client{Timeout: 8 * time.Second}}, nil
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
	q := url.Values{"page": {"1"}, "perPage": {strconv.Itoa(maxRecipes)}, "orderBy": {"name"}, "orderDirection": {"asc"}}
	var page struct {
		Items []summary `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/recipes", q, nil, &page); err != nil {
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
	err := c.do(ctx, http.MethodGet, "/api/recipes/"+url.PathEscape(slug), nil, nil, &r)
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
	if group := c.group(ctx); group != "" {
		out.URL = c.base.JoinPath("g", group, "r", r.Slug).String()
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
	if err := c.do(ctx, http.MethodPost, "/api/households/shopping/lists/"+url.PathEscape(id)+"/recipe", nil, body, nil); err != nil {
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
		if err := c.do(ctx, http.MethodGet, "/api/households/shopping/lists/"+url.PathEscape(c.listID), nil, nil, &l); err != nil {
			return "", "", err
		}
		return l.ID, l.Name, nil
	}
	var page struct {
		Items []list `json:"items"`
	}
	q := url.Values{"page": {"1"}, "perPage": {"1"}, "orderBy": {"updatedAt"}, "orderDirection": {"desc"}}
	if err := c.do(ctx, http.MethodGet, "/api/households/shopping/lists", q, nil, &page); err != nil {
		return "", "", err
	}
	if len(page.Items) == 0 {
		return "", "", fmt.Errorf("%w: Mealie has no shopping list", domain.ErrNotFound)
	}
	return page.Items[0].ID, page.Items[0].Name, nil
}

// group returns the group's slug for recipe links, or "" if Mealie won't say.
func (c *Client) group(ctx context.Context) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.groupSlug == "" {
		var g struct {
			Slug string `json:"slug"`
		}
		if c.do(ctx, http.MethodGet, "/api/groups/self", nil, nil, &g) == nil {
			c.groupSlug = g.Slug
		}
	}
	return c.groupSlug
}

// do sends one request. Mealie being down, slow or refusing the token is domain.ErrUnavailable;
// a 404 is domain.ErrNotFound.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.base.JoinPath(path)
	u.RawQuery = query.Encode()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: mealie %s %s: %w", domain.ErrUnavailable, method, path, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: mealie %s", domain.ErrNotFound, path)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode >= 500:
		return fmt.Errorf("%w: mealie %s %s: %s", domain.ErrUnavailable, method, path, resp.Status)
	case resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mealie %s %s: %s: %s", method, path, resp.Status, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out); err != nil {
		return fmt.Errorf("%w: mealie %s: %w", domain.ErrUnavailable, path, err)
	}
	return nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
