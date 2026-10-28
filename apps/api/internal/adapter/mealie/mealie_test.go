package mealie_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/mealie"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

const soup = `{
  "id": "c9fde254-ea2b-43ab-b693-d2860e13b24e", "slug": "soup", "name": "Pilzsuppe", "description": "Cremig",
  "totalTime": null, "recipeYield": "", "recipeServings": 2,
  "recipeCategory": [{"name": "Suppe"}], "tags": [{"name": "Pilze"}],
  "recipeIngredient": [
    {"referenceId": "a", "display": "450 Gramm Champignon", "quantity": 450, "food": {"name": "Champignon"}},
    {"referenceId": "b", "display": "", "originalText": "1 große Zwiebel"},
    {"referenceId": "c", "display": "Salz"}
  ],
  "recipeInstructions": [{"text": "Zwiebel dünsten."}, {"text": "  "}, {"text": "Pürieren."}]
}`

// fake is a Mealie MCP server: one tool per endpoint, answering with Mealie's JSON as text and
// reporting Mealie's errors the way the real server does ("HTTP error 404: ...").
type fake struct {
	posted  []map[string]any
	down    bool
	restart func() // forgets every MCP session, like a server restart
}

func (f *fake) server(t *testing.T) *httptest.Server {
	t.Helper()
	s := mcp.NewServer(&mcp.Implementation{Name: "mealie", Version: "test"}, nil)
	tool := func(name string, h func(args map[string]any) (string, int)) {
		s.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args map[string]any
			_ = json.Unmarshal(req.Params.Arguments, &args)
			if f.down {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "HTTP error 502: Bad Gateway"}}}, nil
			}
			body, status := h(args)
			if status != 0 {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "HTTP error " + strconv.Itoa(status) + body}}}, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: body}}}, nil
		})
	}
	tool("list_recipes", func(args map[string]any) (string, int) {
		if args["perPage"] == nil {
			t.Error("listing without perPage")
		}
		return `{"items": [` + soup + `]}`, 0
	})
	tool("get_recipes_by_slug", func(args map[string]any) (string, int) {
		if args["slug"] != "soup" {
			return ": Not Found - {'detail': {'message': 'No Entry Found'}}", http.StatusNotFound
		}
		return soup, 0
	})
	tool("list_groups_self", func(map[string]any) (string, int) { return `{"slug": "home"}`, 0 })
	tool("list_households_shopping_lists", func(args map[string]any) (string, int) {
		if args["orderBy"] != "updatedAt" {
			t.Error("lists not ordered by last change")
		}
		return `{"items": [{"id": "l1", "name": "Woche"}]}`, 0
	})
	tool("create_households_shopping_lists_by_item_recipe", func(args map[string]any) (string, int) {
		if args["item_id"] != "l1" {
			t.Errorf("list %v", args["item_id"])
		}
		raw, _ := json.Marshal(args["body"])
		_ = json.Unmarshal(raw, &f.posted)
		return `{}`, 0
	})
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil)
	f.restart = func() { h = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRecipes(t *testing.T) {
	f := &fake{}
	c, err := mealie.New(f.server(t).URL, "tok", "https://mealie.example/", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	all, err := c.Recipes(ctx)
	if err != nil || len(all) != 1 || all[0].Slug != "soup" || len(all[0].Tags) != 2 || all[0].Tags[0] != "Suppe" {
		t.Fatalf("recipes: %+v, %v", all, err)
	}
	r, err := c.Recipe(ctx, "soup")
	if err != nil {
		t.Fatal(err)
	}
	if r.Servings != "2" || len(r.Ingredients) != 3 || r.Ingredients[1].Text != "1 große Zwiebel" || len(r.Steps) != 2 {
		t.Fatalf("recipe: %+v", r)
	}
	if r.URL != "https://mealie.example/g/home/r/soup" {
		t.Fatalf("url: %q", r.URL)
	}
	if _, err := c.Recipe(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown recipe: want ErrNotFound, got %v", err)
	}
}

func TestAddToShoppingList(t *testing.T) {
	f := &fake{}
	c, _ := mealie.New(f.server(t).URL, "tok", "", "")
	ctx := context.Background()

	added, err := c.AddToShoppingList(ctx, "soup", []string{"a", "c", "zzz"})
	if err != nil || added != (domain.ShoppingListAdded{Added: 2, List: "Woche"}) {
		t.Fatalf("add: %+v, %v", added, err)
	}
	if len(f.posted) != 1 || f.posted[0]["recipeId"] != "c9fde254-ea2b-43ab-b693-d2860e13b24e" {
		t.Fatalf("posted: %+v", f.posted)
	}
	ings, _ := f.posted[0]["recipeIngredients"].([]any)
	first, _ := ings[0].(map[string]any)
	if len(ings) != 2 || first["referenceId"] != "a" || first["quantity"] != 450.0 || first["food"] == nil {
		t.Fatalf("ingredients not passed on as Mealie stores them: %+v", ings)
	}
	if _, err := c.AddToShoppingList(ctx, "soup", []string{"zzz"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("foreign ingredients: want ErrInvalid, got %v", err)
	}
}

func TestReconnectsAfterTheServerForgotTheSession(t *testing.T) {
	f := &fake{}
	c, _ := mealie.New(f.server(t).URL, "tok", "", "")
	ctx := context.Background()
	if _, err := c.Recipes(ctx); err != nil {
		t.Fatal(err)
	}
	f.restart()
	if _, err := c.Recipes(ctx); err != nil {
		t.Fatalf("after restart: %v", err)
	}
}

func TestOutagesAreUnavailable(t *testing.T) {
	f := &fake{}
	srv := f.server(t)
	ctx := context.Background()

	bad, _ := mealie.New(srv.URL, "wrong", "", "")
	if _, err := bad.Recipes(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("bad token: want ErrUnavailable, got %v", err)
	}
	f.down = true
	c, _ := mealie.New(srv.URL, "tok", "", "")
	if _, err := c.Recipes(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("down: want ErrUnavailable, got %v", err)
	}
	srv.Close()
	if _, err := c.Recipes(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("unreachable: want ErrUnavailable, got %v", err)
	}
	if _, err := mealie.New("mealie.local", "tok", "", ""); err == nil {
		t.Fatal("a URL without scheme is accepted")
	}
	if r, err := c.Recipe(ctx, "soup"); err == nil || r.URL != "" {
		t.Fatalf("closed server: %+v, %v", r, err)
	}
}
