package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// The assistant's tools are its whitelist: it can read and add entries, tick reminders off, look
// recipes up, read and add to the shopping list, add expenses and read the money, list, add and
// tick chores off, read and add to the household wishlist, keep gift ideas, and read the home
// and switch its scenes and lights. It can't delete anything, and every call goes through the
// services with the person as the actor.
var tools = []domain.Tool{
	{
		Name:     "list_entries",
		ReadOnly: true,
		Description: "List the household's reminders, calendar events or notes that the person can see. " +
			"Events are upcoming ones, a repeating event at its next time. Reminders include those done in the last week. " +
			"A note that opens later shows only that it exists and when it opens.",
		Schema: object(map[string]any{"kind": enum("reminder", "event", "note")}, "kind"),
	},
	{
		Name: "add_reminder",
		Description: "Add a reminder. Times are local, as YYYY-MM-DDTHH:MM. A repeating reminder needs a due time. " +
			"\"for\" is the name of the member it is for; leave it out for everyone. Only a shared reminder can be for someone.",
		Schema: object(map[string]any{
			"text":       str("What to remember, at most 200 characters."),
			"visibility": visibilityEnum("private", "shared"),
			"due":        str("When it is due."),
			"repeat":     repeatEnum,
			"for":        str("Name of the member it is for."),
			"bill":       billSchema,
		}, "text", "visibility"),
	},
	{
		Name: "add_event",
		Description: "Add a calendar event. Times are local, as YYYY-MM-DDTHH:MM, or YYYY-MM-DD for an all-day event. " +
			"alert_minutes is how long before the start a notification goes out.",
		Schema: object(map[string]any{
			"text":          str("Title, at most 200 characters."),
			"visibility":    visibilityEnum("private", "shared"),
			"starts":        str("Start."),
			"ends":          str("End, optional."),
			"all_day":       map[string]any{"type": "boolean"},
			"alert_minutes": map[string]any{"type": "integer", "minimum": 0, "maximum": domain.MaxAlertMinutes},
			"repeat":        repeatEnum,
		}, "text", "visibility", "starts"),
	},
	{
		Name: "add_note",
		Description: "Add a note. A \"reveal\" note is visible to the others only from reveal_at on (local, YYYY-MM-DDTHH:MM); " +
			"until then they only see that it exists.",
		Schema: object(map[string]any{
			"text":       str("The note in Markdown, at most 20000 characters; start it with a \"# Title\" line."),
			"visibility": visibilityEnum("private", "shared", "reveal"),
			"reveal_at":  str("When a reveal note opens."),
		}, "text", "visibility"),
	},
	{
		Name:        "complete_reminder",
		Description: "Tick a reminder off by its id from list_entries. A repeating one moves on to its next time.",
		Schema:      object(map[string]any{"id": str("The reminder's id.")}, "id"),
	},
	{
		Name:        "list_chores",
		ReadOnly:    true,
		Description: "List the household's open chores, ordered by when they are due.",
		Schema:      object(map[string]any{}),
	},
	{
		Name: "add_chore",
		Description: "Add a household chore. due is YYYY-MM-DD, today when left out. points is the effort from 1 (small) to 3 (big), " +
			"1 when left out. \"for\" is the name of the member whose turn it is; leave it out for anyone (or the person, when rotate is true). " +
			"When rotate is true, members take turns each time it is done.",
		Schema: object(map[string]any{
			"title":  str("What needs doing, at most 100 characters."),
			"points": map[string]any{"type": "integer", "minimum": 1, "maximum": domain.MaxChorePoints, "description": "Effort, 1 to 3."},
			"repeat": repeatEnum,
			"rotate": map[string]any{"type": "boolean", "description": "Whether members take turns."},
			"for":    str("Name of the member whose turn it is."),
			"due":    str("When it is due, YYYY-MM-DD."),
		}, "title"),
	},
	{
		Name: "complete_chore",
		Description: "Tick a chore off as done by the person, by its id from list_chores. " +
			"A repeating chore moves on to its next due day (and to the next member when it rotates).",
		Schema: object(map[string]any{"id": str("The chore's id.")}, "id"),
	},
	{
		Name:     "search_recipes",
		ReadOnly: true,
		Description: "Search the household's recipe collection by words in the name, tags or description " +
			"(the recipes are mostly German, so search in German too). Returns up to 10 matches; an empty query returns a random few.",
		Schema: object(map[string]any{"query": str("Search words.")}),
	},
	{
		Name:        "get_recipe",
		ReadOnly:    true,
		Description: "Get one recipe with its ingredients and steps, by its slug from search_recipes.",
		Schema:      object(map[string]any{"slug": str("The recipe's slug.")}, "slug"),
	},
	{
		Name:        "list_shopping",
		ReadOnly:    true,
		Description: "List the household's shared shopping list; done items are ticked off but not removed yet.",
		Schema:      object(map[string]any{}),
	},
	{
		Name: "add_to_shopping_list",
		Description: "Put items on the household's shared shopping list, one item per entry, e.g. \"200 g Feta\". " +
			"recipe is the name of the recipe they are for, if any.",
		Schema: object(map[string]any{
			"items":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "maxItems": 50},
			"recipe": str("Name of the recipe."),
		}, "items"),
	},
	{
		Name:     "list_wishes",
		ReadOnly: true,
		Description: "List the household's shared wishlist (things the members would like for the home, not the shopping list " +
			"or private birthday gift ideas), most wanted first.",
		Schema: object(map[string]any{}),
	},
	{
		Name: "add_wish",
		Description: "Add an entry to the household's shared wishlist (something for the home, not the grocery shopping list " +
			"or a private birthday gift idea). price is in euros, optional; hearts is how much the person wants it, from 1 to 3, optional.",
		Schema: object(map[string]any{
			"title":  str("What the wish is, at most 100 characters."),
			"note":   str("Notes or details, at most 500 characters."),
			"url":    str("Web link (http or https) to see or buy it."),
			"price":  map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": domain.MaxWishPriceCents / 100, "description": "Price in euros, e.g. 49.9."},
			"hearts": map[string]any{"type": "integer", "minimum": 1, "maximum": domain.MaxWishHearts, "description": "How much the person wants it, 1 to 3."},
		}, "title"),
	},
	{
		Name: "add_expense",
		Description: "Write down money one member paid for the household, in euros. It is split evenly between all members " +
			"unless shares say otherwise; shares must add up to the amount. paid_by is a member's name; leave it out when the person paid. " +
			"day is YYYY-MM-DD, today when left out.",
		Schema: object(map[string]any{
			"amount":   map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": domain.MaxExpenseCents / 100, "description": "Euros, e.g. 12.5."},
			"category": categoryEnum,
			"note":     str("What it was for, at most 200 characters."),
			"paid_by":  str("Name of the member who paid."),
			"day":      str("The day it was spent."),
			"shares": map[string]any{
				"type": "object", "description": "Euros per member name, when not split evenly.",
				"additionalProperties": map[string]any{"type": "number", "minimum": 0},
			},
		}, "amount", "category"),
	},
	{
		Name:     "get_money",
		ReadOnly: true,
		Description: "Read where the members stand with money (who owes whom how much) and this month's expenses " +
			"and settlements, with what was spent per category. month is YYYY-MM for another month.",
		Schema: object(map[string]any{"month": str("The month, YYYY-MM.")}),
	},
	{
		Name:     "list_gift_ideas",
		ReadOnly: true,
		Description: "List the gift ideas the person has kept for the other members, and when those members' birthdays are. " +
			"The ideas are private: only the person's own are ever shown, never anyone else's, and the assistant must never " +
			"hint at them to anyone else.",
		Schema: object(map[string]any{}),
	},
	{
		Name: "add_gift_idea",
		Description: "Keep a private gift idea for another member, e.g. for their birthday. Only the person who asked can ever see it. " +
			"\"for\" is the name of the member it is for.",
		Schema: object(map[string]any{
			"for":  str("Name of the member the gift is for."),
			"text": str("The idea, at most 200 characters."),
		}, "for", "text"),
	},
	{
		Name:     "get_home",
		ReadOnly: true,
		Description: "Read the home: its rooms with their scenes, lights (on or off) and readings such as temperature " +
			"or an open window. Only the person's own connection is used.",
		Schema: object(map[string]any{}),
	},
	{
		Name:        "activate_scene",
		Description: "Turn a scene on, by its id from get_home.",
		Schema:      object(map[string]any{"id": str("The scene's id, scene.*.")}, "id"),
	},
	{
		Name:        "set_light",
		Description: "Switch a light on or off, by its id from get_home.",
		Schema:      object(map[string]any{"id": str("The light's id, light.*."), "on": map[string]any{"type": "boolean"}}, "id", "on"),
	},
}

var repeatEnum = enum("never", "daily", "weekly", "monthly", "yearly")

var categoryEnum = func() map[string]any {
	values := make([]string, len(domain.ExpenseCategories))
	for i, c := range domain.ExpenseCategories {
		values[i] = string(c)
	}
	return enum(values...)
}()

var billSchema = object(map[string]any{
	"amount":   map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": domain.MaxExpenseCents / 100, "description": "Euros."},
	"category": categoryEnum,
	"paid_by":  str("Name of the member who pays it; leave out for whoever ticks it off."),
}, "amount", "category")

func object(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func str(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func enum(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func visibilityEnum(values ...string) map[string]any {
	e := enum(values...)
	e["description"] = "private: only the person; shared: every member; reveal: others see it later."
	return e
}

// toolbox runs the tools for one person in one household.
type toolbox struct {
	s           *Assistant
	actor       domain.User
	householdID uuid.UUID
	members     []domain.Member
}

func (t *toolbox) run(ctx context.Context, call domain.ToolCall) (string, error) {
	out, err := t.call(ctx, call)
	if err != nil {
		return "", t.explain(ctx, call.Name, err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (t *toolbox) call(ctx context.Context, call domain.ToolCall) (any, error) {
	switch call.Name {
	case "list_entries":
		var in struct {
			Kind domain.EntryKind `json:"kind"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if in.Kind != domain.KindReminder && in.Kind != domain.KindEvent && in.Kind != domain.KindNote {
			return nil, fmt.Errorf("%w: unknown kind", domain.ErrInvalid)
		}
		es, err := t.s.entries.List(ctx, t.actor, t.householdID, in.Kind)
		if err != nil {
			return nil, err
		}
		out := make([]entryView, len(es))
		for i, e := range es {
			out[i] = t.view(e)
		}
		return out, nil
	case "add_reminder":
		var in struct {
			Text       string            `json:"text"`
			Visibility domain.Visibility `json:"visibility"`
			Due        string            `json:"due"`
			Repeat     domain.Repeat     `json:"repeat"`
			For        string            `json:"for"`
			Bill       *struct {
				Amount   float64                `json:"amount"`
				Category domain.ExpenseCategory `json:"category"`
				PaidBy   string                 `json:"paid_by"`
			} `json:"bill"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		f := domain.EntryFields{Text: in.Text, Visibility: in.Visibility, Repeat: in.Repeat}
		var err error
		if f.DueAt, err = t.time(in.Due); err != nil {
			return nil, err
		}
		if in.For != "" {
			if f.AssigneeID, err = t.member(in.For); err != nil {
				return nil, err
			}
		}
		if b := in.Bill; b != nil {
			f.Bill = &domain.Bill{Cents: cents(b.Amount), Category: b.Category}
			if b.PaidBy != "" {
				if f.Bill.PaidBy, err = t.member(b.PaidBy); err != nil {
					return nil, err
				}
			}
		}
		return t.add(ctx, domain.KindReminder, f)
	case "add_event":
		var in struct {
			Text         string            `json:"text"`
			Visibility   domain.Visibility `json:"visibility"`
			Starts       string            `json:"starts"`
			Ends         string            `json:"ends"`
			AllDay       bool              `json:"all_day"`
			AlertMinutes *int              `json:"alert_minutes"`
			Repeat       domain.Repeat     `json:"repeat"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		f := domain.EntryFields{Text: in.Text, Visibility: in.Visibility, AllDay: in.AllDay, AlertMinutes: in.AlertMinutes, Repeat: in.Repeat}
		var err error
		if f.StartsAt, err = t.time(in.Starts); err != nil {
			return nil, err
		}
		if f.EndsAt, err = t.time(in.Ends); err != nil {
			return nil, err
		}
		return t.add(ctx, domain.KindEvent, f)
	case "add_note":
		var in struct {
			Text       string            `json:"text"`
			Visibility domain.Visibility `json:"visibility"`
			RevealAt   string            `json:"reveal_at"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		f := domain.EntryFields{Text: in.Text, Visibility: in.Visibility}
		var err error
		if f.RevealAt, err = t.time(in.RevealAt); err != nil {
			return nil, err
		}
		return t.add(ctx, domain.KindNote, f)
	case "complete_reminder":
		var in struct {
			ID uuid.UUID `json:"id"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		e, err := t.s.entries.SetDone(ctx, t.actor, in.ID, true)
		if err != nil {
			return nil, err
		}
		return t.view(e), nil
	case "list_chores":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		return t.chores(ctx)
	case "add_chore":
		var in struct {
			Title  string        `json:"title"`
			Points int           `json:"points"`
			Repeat domain.Repeat `json:"repeat"`
			Rotate bool          `json:"rotate"`
			For    string        `json:"for"`
			Due    string        `json:"due"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.addChore(ctx, in.Title, in.Points, in.Repeat, in.Rotate, in.For, in.Due)
	case "complete_chore":
		var in struct {
			ID uuid.UUID `json:"id"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.completeChore(ctx, in.ID)
	case "search_recipes":
		var in struct {
			Query string `json:"query"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		rs, err := t.s.cook.Find(ctx, t.actor, t.householdID, in.Query, recipeMatches)
		if err != nil {
			return nil, err
		}
		out := make([]recipeView, len(rs))
		for i, r := range rs {
			out[i] = recipeView{Slug: r.Slug, Name: r.Name, Tags: r.Tags, Time: r.TotalTime, Description: clipText(r.Description, 200)}
		}
		return out, nil
	case "get_recipe":
		var in struct {
			Slug string `json:"slug"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		r, err := t.s.cook.Recipe(ctx, t.actor, t.householdID, in.Slug)
		if err != nil {
			return nil, err
		}
		ingredients := make([]string, len(r.Ingredients))
		for i, in := range r.Ingredients {
			ingredients[i] = in.Text
		}
		return struct {
			recipeView
			Servings    string   `json:"servings,omitempty"`
			Ingredients []string `json:"ingredients"`
			Steps       []string `json:"steps"`
		}{recipeView{Slug: r.Slug, Name: r.Name, Tags: r.Tags, Time: r.TotalTime, Description: r.Description}, r.Servings, ingredients, r.Steps}, nil
	case "list_shopping":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		items, err := t.s.shopping.Items(ctx, t.actor, t.householdID)
		if err != nil {
			return nil, err
		}
		return shoppingView(items), nil
	case "add_to_shopping_list":
		var in struct {
			Items  []string `json:"items"`
			Recipe string   `json:"recipe"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		items, err := t.s.shopping.Add(ctx, t.actor, t.householdID, in.Items, strings.TrimSpace(in.Recipe))
		if err != nil {
			return nil, err
		}
		return shoppingView(items), nil
	case "list_wishes":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		return t.wishes(ctx)
	case "add_wish":
		var in struct {
			Title  string   `json:"title"`
			Note   string   `json:"note"`
			URL    string   `json:"url"`
			Price  *float64 `json:"price"`
			Hearts *int     `json:"hearts"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.addWish(ctx, in.Title, in.Note, in.URL, in.Price, in.Hearts)
	case "add_expense":
		var in struct {
			Amount   float64                `json:"amount"`
			Category domain.ExpenseCategory `json:"category"`
			Note     string                 `json:"note"`
			PaidBy   string                 `json:"paid_by"`
			Day      string                 `json:"day"`
			Shares   map[string]float64     `json:"shares"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.addExpense(ctx, in.Amount, in.Category, in.Note, in.PaidBy, in.Day, in.Shares)
	case "list_gift_ideas":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		return t.giftIdeas(ctx)
	case "add_gift_idea":
		var in struct {
			For  string `json:"for"`
			Text string `json:"text"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.addGiftIdea(ctx, in.For, in.Text)
	case "get_money":
		var in struct {
			Month string `json:"month"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.money(ctx, in.Month)
	case "get_home":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		h, err := t.s.home.Get(ctx, t.actor, t.householdID)
		if err != nil {
			return nil, err
		}
		return homeView(h), nil
	case "activate_scene":
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(in.ID, "scene.") {
			return nil, domain.ErrNotFound
		}
		if err := t.s.home.ActivateScene(ctx, t.actor, t.householdID, in.ID); err != nil {
			return nil, err
		}
		return map[string]bool{"done": true}, nil
	case "set_light":
		var in struct {
			ID string `json:"id"`
			On bool   `json:"on"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.setLight(ctx, in.ID, in.On)
	}
	return t.callMore(ctx, call)
}

func (t *toolbox) add(ctx context.Context, kind domain.EntryKind, f domain.EntryFields) (any, error) {
	e, err := t.s.entries.Create(ctx, t.actor, t.householdID, kind, f)
	if err != nil {
		return nil, err
	}
	return t.view(e), nil
}

type choreView struct {
	ID     uuid.UUID     `json:"id"`
	Title  string        `json:"title"`
	Points int           `json:"points"`
	Repeat domain.Repeat `json:"repeat,omitempty"`
	Rotate bool          `json:"rotate,omitempty"`
	For    string        `json:"for,omitempty"`
	Due    string        `json:"due"`
}

func (t *toolbox) choreView(c domain.Chore) choreView {
	v := choreView{ID: c.ID, Title: c.Title, Points: c.Points, Rotate: c.Rotate, Due: c.DueOn.Format(time.DateOnly)}
	if c.Repeat != domain.RepeatNever {
		v.Repeat = c.Repeat
	}
	if c.AssigneeID != nil {
		v.For = t.name(*c.AssigneeID)
	}
	return v
}

func (t *toolbox) chores(ctx context.Context) (any, error) {
	if t.s.chores == nil {
		return nil, domain.ErrUnavailable
	}
	cs, err := t.s.chores.List(ctx, t.actor, t.householdID)
	if err != nil {
		return nil, err
	}
	out := make([]choreView, len(cs))
	for i, c := range cs {
		out[i] = t.choreView(c)
	}
	return out, nil
}

func (t *toolbox) addChore(ctx context.Context, title string, points int, repeat domain.Repeat, rotate bool, forName, due string) (any, error) {
	if t.s.chores == nil {
		return nil, domain.ErrUnavailable
	}
	if points == 0 {
		points = 1
	}
	f := domain.ChoreFields{Title: title, Points: points, Repeat: repeat, Rotate: rotate}
	if forName != "" {
		id, err := t.member(forName)
		if err != nil {
			return nil, err
		}
		f.AssigneeID = id
	}
	if due != "" {
		d, err := time.Parse(time.DateOnly, strings.TrimSpace(due))
		if err != nil {
			return nil, fmt.Errorf("%w: due %q is not YYYY-MM-DD", domain.ErrInvalid, due)
		}
		f.DueOn = d
	}
	c, err := t.s.chores.Create(ctx, t.actor, t.householdID, f)
	if err != nil {
		return nil, err
	}
	return t.choreView(c), nil
}

func (t *toolbox) completeChore(ctx context.Context, id uuid.UUID) (any, error) {
	if t.s.chores == nil {
		return nil, domain.ErrUnavailable
	}
	d, err := t.s.chores.Complete(ctx, t.actor, t.householdID, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": d.ID, "title": d.Title, "points": d.Points, "day": d.Day.Format(time.DateOnly)}, nil
}

type wishView struct {
	ID     uuid.UUID      `json:"id"`
	Title  string         `json:"title"`
	Note   string         `json:"note,omitempty"`
	URL    string         `json:"url,omitempty"`
	Price  string         `json:"price_eur,omitempty"`
	Hearts int            `json:"hearts"`
	Votes  map[string]int `json:"votes,omitempty"`
	Got    bool           `json:"got,omitempty"`
}

func (t *toolbox) wishView(w domain.Wish) wishView {
	v := wishView{ID: w.ID, Title: w.Title, Note: w.Note, URL: w.URL, Hearts: w.Hearts(), Got: w.GotAt != nil}
	if w.PriceCents != nil {
		v.Price = euros(*w.PriceCents)
	}
	if len(w.Votes) > 0 {
		v.Votes = make(map[string]int, len(w.Votes))
		for _, vote := range w.Votes {
			v.Votes[t.name(vote.UserID)] = vote.Hearts
		}
	}
	return v
}

func (t *toolbox) wishes(ctx context.Context) (any, error) {
	if t.s.wishes == nil {
		return nil, domain.ErrUnavailable
	}
	ws, err := t.s.wishes.List(ctx, t.actor, t.householdID)
	if err != nil {
		return nil, err
	}
	out := make([]wishView, len(ws))
	for i, w := range ws {
		out[i] = t.wishView(w)
	}
	return out, nil
}

func (t *toolbox) addWish(ctx context.Context, title, note, url string, price *float64, hearts *int) (any, error) {
	if t.s.wishes == nil {
		return nil, domain.ErrUnavailable
	}
	if hearts != nil && (*hearts < 1 || *hearts > domain.MaxWishHearts) {
		return nil, domain.ErrWishHearts
	}
	f := domain.WishFields{Title: title, Note: note, URL: url}
	if price != nil {
		c := cents(*price)
		f.PriceCents = &c
	}
	w, err := t.s.wishes.Create(ctx, t.actor, t.householdID, f)
	if err != nil {
		return nil, err
	}
	if hearts != nil {
		if w, err = t.s.wishes.Vote(ctx, t.actor, t.householdID, w.ID, *hearts); err != nil {
			return nil, err
		}
	}
	return t.wishView(w), nil
}

func (t *toolbox) addExpense(ctx context.Context, amount float64, category domain.ExpenseCategory, note, paidBy, day string, shares map[string]float64) (any, error) {
	if t.s.budget == nil {
		return nil, domain.ErrUnavailable
	}
	f := domain.ExpenseFields{Cents: cents(amount), Category: category, Note: note, PaidBy: t.actor.ID}
	if paidBy != "" {
		id, err := t.member(paidBy)
		if err != nil {
			return nil, err
		}
		f.PaidBy = *id
	}
	if day != "" {
		d, err := time.Parse(time.DateOnly, strings.TrimSpace(day))
		if err != nil {
			return nil, fmt.Errorf("%w: day %q is not YYYY-MM-DD", domain.ErrInvalid, day)
		}
		f.Day = d
	}
	for name, amount := range shares {
		id, err := t.member(name)
		if err != nil {
			return nil, err
		}
		f.Shares = append(f.Shares, domain.Share{UserID: *id, Cents: cents(amount)})
	}
	e, err := t.s.budget.Add(ctx, t.actor, t.householdID, f)
	if err != nil {
		return nil, err
	}
	return t.expenseView(e), nil
}

func (t *toolbox) giftIdeas(ctx context.Context) (any, error) {
	if t.s.gifts == nil {
		return nil, domain.ErrUnavailable
	}
	g, err := t.s.gifts.Overview(ctx, t.actor, t.householdID)
	if err != nil {
		return nil, err
	}
	type person struct {
		Name     string `json:"name"`
		Birthday string `json:"birthday,omitempty"`
		Turns    int    `json:"turns,omitempty"` // the age they turn, when their year of birth is known
		InDays   *int   `json:"in_days,omitempty"`
	}
	type idea struct {
		ID    uuid.UUID `json:"id"`
		For   string    `json:"for"`
		Text  string    `json:"text"`
		Given bool      `json:"given,omitempty"`
	}
	out := struct {
		People []person `json:"people"`
		Ideas  []idea   `json:"ideas"`
	}{People: []person{}, Ideas: []idea{}}
	for _, p := range g.People {
		v := person{Name: p.User.Name}
		if p.Birthday != nil {
			v.Birthday = p.Next.Format("2006-01-02")
			v.Turns = p.Birthday.Age(p.Next)
			days := p.Days
			v.InDays = &days
		}
		out.People = append(out.People, v)
	}
	for _, i := range g.Ideas {
		out.Ideas = append(out.Ideas, idea{ID: i.ID, For: t.name(i.ForID), Text: i.Text, Given: i.GivenAt != nil})
	}
	return out, nil
}

func (t *toolbox) addGiftIdea(ctx context.Context, forName, text string) (any, error) {
	if t.s.gifts == nil {
		return nil, domain.ErrUnavailable
	}
	id, err := t.member(forName)
	if err != nil {
		return nil, err
	}
	i, err := t.s.gifts.Add(ctx, t.actor, t.householdID, *id, text)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": i.ID, "for": t.name(i.ForID), "text": i.Text}, nil
}

func (t *toolbox) money(ctx context.Context, month string) (any, error) {
	if t.s.budget == nil {
		return nil, domain.ErrUnavailable
	}
	var m time.Time
	if month != "" {
		var err error
		if m, err = time.Parse("2006-01", strings.TrimSpace(month)); err != nil {
			return nil, fmt.Errorf("%w: month %q is not YYYY-MM", domain.ErrInvalid, month)
		}
	}
	b, err := t.s.budget.Balance(ctx, t.actor, t.householdID)
	if err != nil {
		return nil, err
	}
	em, err := t.s.budget.Month(ctx, t.actor, t.householdID, m)
	if err != nil {
		return nil, err
	}
	standing := make(map[string]string, len(b.Members))
	for _, n := range b.Members {
		standing[t.name(n.UserID)] = euros(n.Cents)
	}
	settle := make([]string, len(b.Transfers))
	for i, x := range b.Transfers {
		settle[i] = fmt.Sprintf("%s pays %s %s EUR", t.name(x.From), t.name(x.To), euros(x.Cents))
	}
	spent := make(map[string]string, len(em.Categories))
	for _, c := range em.Categories {
		spent[string(c.Category)] = euros(c.Cents)
	}
	expenses := make([]expenseView, len(em.Expenses))
	for i, e := range em.Expenses {
		expenses[i] = t.expenseView(e)
	}
	return map[string]any{
		"standing_eur":  standing,
		"to_settle":     settle,
		"month":         em.Month.Format("2006-01"),
		"spent_eur":     euros(em.Total),
		"by_category":   spent,
		"expenses":      expenses,
		"earlier_month": formatMonth(em.Earlier),
	}, nil
}

type expenseView struct {
	Kind     domain.ExpenseKind     `json:"kind"`
	Amount   string                 `json:"amount_eur"`
	Category domain.ExpenseCategory `json:"category,omitempty"`
	Note     string                 `json:"note,omitempty"`
	PaidBy   string                 `json:"paid_by"`
	Day      string                 `json:"day"`
	Shares   map[string]string      `json:"shares_eur"`
}

func (t *toolbox) expenseView(e domain.Expense) expenseView {
	v := expenseView{Kind: e.Kind, Amount: euros(e.Cents), Note: e.Note, PaidBy: t.name(e.PaidBy), Day: e.Day.Format(time.DateOnly), Shares: map[string]string{}}
	if e.Kind == domain.KindExpense {
		v.Category = e.Category
	}
	for _, sh := range e.Shares {
		v.Shares[t.name(sh.UserID)] = euros(sh.Cents)
	}
	return v
}

// cents turns euros as the model writes them into cents.
func cents(euros float64) int64 { return int64(math.Round(euros * 100)) }

func euros(cents int64) string { return fmt.Sprintf("%.2f", float64(cents)/100) }

func formatMonth(m time.Time) string {
	if m.IsZero() {
		return ""
	}
	return m.Format("2006-01")
}

// setLight switches only when the light isn't in that state yet; the port can only toggle.
func (t *toolbox) setLight(ctx context.Context, id string, on bool) (any, error) {
	if !strings.HasPrefix(id, "light.") {
		return nil, domain.ErrNotFound
	}
	h, err := t.s.home.Get(ctx, t.actor, t.householdID)
	if err != nil {
		return nil, err
	}
	if !h.Connected {
		return nil, errNotConnected
	}
	for _, l := range h.Lights {
		if l.ID != id {
			continue
		}
		if l.On != on {
			if l, err = t.s.home.ToggleLight(ctx, t.actor, t.householdID, id); err != nil {
				return nil, err
			}
		}
		return map[string]any{"id": l.ID, "name": l.Name, "on": l.On}, nil
	}
	return nil, domain.ErrNotFound
}

var errNotConnected = errors.New("the person hasn't connected their home yet; they can do that in the Home tab")

// explain turns an error into what the model is told; failures it can't fix are logged instead.
func (t *toolbox) explain(ctx context.Context, tool string, err error) error {
	var text domain.TextError
	switch {
	case errors.Is(err, errNotConnected):
		return err
	case errors.Is(err, domain.ErrNotFound):
		return errors.New("not found")
	case errors.As(err, &text), errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrConflict):
		return err
	case errors.Is(err, domain.ErrForbidden):
		return errors.New("only its author can change that")
	case errors.Is(err, domain.ErrUnavailable):
		slog.WarnContext(ctx, "assistant tool unavailable", "tool", tool, "err", err)
		return errors.New("that part of the app isn't available right now")
	}
	slog.ErrorContext(ctx, "assistant tool failed", "tool", tool, "err", err)
	return errNoAnswer
}

// decode reads a tool's input strictly, so a misnamed field is an error the model can correct.
func decode(raw json.RawMessage, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrInvalid, err) //nolint:errorlint // the decode error is only text for the model
	}
	return nil
}

// time reads a local time as the model writes it; "" is no time.
func (t *toolbox) time(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil //nolint:nilnil // no time given
	}
	if v, err := time.Parse(time.RFC3339, s); err == nil {
		return &v, nil
	}
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if v, err := time.ParseInLocation(layout, s, t.s.loc); err == nil {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("%w: time %q is not YYYY-MM-DDTHH:MM or YYYY-MM-DD", domain.ErrInvalid, s)
}

// member finds a member by name, ignoring case; the model knows members only by name.
func (t *toolbox) member(name string) (*uuid.UUID, error) {
	var names []string
	for _, m := range t.members {
		if strings.EqualFold(strings.TrimSpace(name), m.User.Name) {
			return &m.User.ID, nil
		}
		names = append(names, m.User.Name)
	}
	return nil, fmt.Errorf("%w: no member is called %q; the members are %s", domain.ErrInvalid, name, strings.Join(names, ", "))
}

func (t *toolbox) name(id uuid.UUID) string {
	for _, m := range t.members {
		if m.User.ID == id {
			if id == t.actor.ID {
				return m.User.Name + " (you)"
			}
			return m.User.Name
		}
	}
	return "a former member"
}

type entryView struct {
	ID         uuid.UUID         `json:"id"`
	Text       string            `json:"text,omitempty"`
	Sealed     bool              `json:"sealed,omitempty"`
	Visibility domain.Visibility `json:"visibility"`
	RevealAt   string            `json:"reveal_at,omitempty"`
	Author     string            `json:"author"`
	For        string            `json:"for,omitempty"`
	Due        string            `json:"due,omitempty"`
	Starts     string            `json:"starts,omitempty"`
	Ends       string            `json:"ends,omitempty"`
	AllDay     bool              `json:"all_day,omitempty"`
	Alert      *int              `json:"alert_minutes,omitempty"`
	Repeat     domain.Repeat     `json:"repeat,omitempty"`
	Done       bool              `json:"done,omitempty"`
}

func (t *toolbox) view(e domain.Entry) entryView {
	v := entryView{
		ID: e.ID, Text: e.Text, Sealed: e.Sealed, Visibility: e.Visibility, Author: t.name(e.AuthorID),
		RevealAt: t.local(e.RevealAt), Starts: t.local(e.StartsAt), Ends: t.local(e.EndsAt), AllDay: e.AllDay,
		Alert: e.AlertMinutes, Done: e.DoneAt != nil,
	}
	if e.Kind == domain.KindReminder {
		v.Due = t.local(e.DueAt)
	}
	if e.Repeat != domain.RepeatNever {
		v.Repeat = e.Repeat
	}
	if e.AssigneeID != nil {
		v.For = t.name(*e.AssigneeID)
	}
	return v
}

func (t *toolbox) local(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.In(t.s.loc).Format("Mon 2006-01-02 15:04")
}

type recipeView struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Tags        []string `json:"tags,omitempty"`
	Time        string   `json:"time,omitempty"`
	Description string   `json:"description,omitempty"`
}

// recipeMatches is how many recipes one search returns.
const recipeMatches = 10

func shoppingView(items []domain.ShoppingItem) []map[string]any {
	out := make([]map[string]any, len(items))
	for i, it := range items {
		v := map[string]any{"id": it.ID, "text": it.Text}
		if it.Recipe != "" {
			v["recipe"] = it.Recipe
		}
		if it.DoneAt != nil {
			v["done"] = true
		}
		out[i] = v
	}
	return out
}

type roomView struct {
	Name     string           `json:"name"`
	Scenes   []map[string]any `json:"scenes,omitempty"`
	Lights   []map[string]any `json:"lights,omitempty"`
	Readings []map[string]any `json:"readings,omitempty"`
}

func homeView(h domain.Home) any {
	if !h.Connected {
		return map[string]any{"connected": false, "note": errNotConnected.Error()}
	}
	rooms := map[string]*roomView{"": {Name: "Elsewhere"}}
	order := []string{}
	for _, r := range h.Rooms {
		rooms[r.ID] = &roomView{Name: r.Name}
		order = append(order, r.ID)
	}
	order = append(order, "")
	in := func(id string) *roomView {
		if r, ok := rooms[id]; ok {
			return r
		}
		return rooms[""]
	}
	for _, s := range h.Scenes {
		r := in(s.Room)
		r.Scenes = append(r.Scenes, map[string]any{"id": s.ID, "name": s.Name})
	}
	for _, l := range h.Lights {
		r := in(l.Room)
		r.Lights = append(r.Lights, map[string]any{"id": l.ID, "name": l.Name, "on": l.On})
	}
	for _, x := range h.Readings {
		r := in(x.Room)
		v := map[string]any{"name": x.Name, "kind": x.Kind}
		if x.Kind.Measured() {
			v["value"], v["unit"] = x.Value, x.Unit
		} else {
			v["active"] = x.On
		}
		r.Readings = append(r.Readings, v)
	}
	out := []roomView{}
	for _, id := range order {
		if r := rooms[id]; len(r.Scenes)+len(r.Lights)+len(r.Readings) > 0 {
			out = append(out, *r)
		}
	}
	return map[string]any{"connected": true, "rooms": out}
}

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
