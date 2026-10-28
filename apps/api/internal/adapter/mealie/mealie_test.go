package mealie_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

type fake struct {
	posted []map[string]any
	down   bool
}

func (f *fake) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/recipes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("perPage") == "" {
			t.Error("listing without perPage")
		}
		_, _ = w.Write([]byte(`{"items": [` + soup + `]}`))
	})
	mux.HandleFunc("GET /api/recipes/soup", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(soup)) })
	mux.HandleFunc("GET /api/groups/self", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"slug": "home"}`)) })
	mux.HandleFunc("GET /api/households/shopping/lists", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("orderBy") != "updatedAt" {
			t.Error("lists not ordered by last change")
		}
		_, _ = w.Write([]byte(`{"items": [{"id": "l1", "name": "Woche"}]}`))
	})
	mux.HandleFunc("POST /api/households/shopping/lists/l1/recipe", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&f.posted)
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.down {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRecipes(t *testing.T) {
	f := &fake{}
	c, err := mealie.New(f.server(t).URL+"/", "tok", "")
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
	if !strings.HasSuffix(r.URL, "/g/home/r/soup") {
		t.Fatalf("url: %q", r.URL)
	}
	if _, err := c.Recipe(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown recipe: want ErrNotFound, got %v", err)
	}
}

func TestAddToShoppingList(t *testing.T) {
	f := &fake{}
	c, _ := mealie.New(f.server(t).URL, "tok", "")
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

func TestOutagesAreUnavailable(t *testing.T) {
	f := &fake{}
	srv := f.server(t)
	ctx := context.Background()

	bad, _ := mealie.New(srv.URL, "wrong", "")
	if _, err := bad.Recipes(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("bad token: want ErrUnavailable, got %v", err)
	}
	f.down = true
	c, _ := mealie.New(srv.URL, "tok", "")
	if _, err := c.Recipes(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("down: want ErrUnavailable, got %v", err)
	}
	srv.Close()
	if _, err := c.Recipes(ctx); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("unreachable: want ErrUnavailable, got %v", err)
	}
	if _, err := mealie.New("mealie.local", "tok", ""); err == nil {
		t.Fatal("a URL without scheme is accepted")
	}
}
