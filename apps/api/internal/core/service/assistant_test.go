package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type assistantFixture struct {
	*entryFixture
	llm       *memory.Assistant
	smart     *memory.SmartHome
	home      *service.Home
	assistant *service.Assistant
}

func newAssistantFixture(t *testing.T) *assistantFixture {
	t.Helper()
	f := &assistantFixture{entryFixture: newEntryFixture(t), llm: &memory.Assistant{}, smart: smartHome()}
	f.home = service.NewHome(f.store, f.store, memory.NewSessions(), f.smart)
	shopping := service.NewShopping(f.store, f.store, nil)
	cook := service.NewCook(f.store, memory.NewRecipeBook(recipes()...), nil, shopping).WithPlan(f.store).WithTaste(f.store, f.store)
	// The same time zone as the entries, as in the app.
	f.assistant = service.NewAssistant(f.store, f.store, f.llm, f.entries, cook, shopping, f.home, time.UTC)
	return f
}

// say asks the assistant as u in a new conversation, or in c, and returns its answer.
func (f *assistantFixture) say(t *testing.T, u domain.User, c *uuid.UUID, text string) (domain.Conversation, string) {
	t.Helper()
	ctx := context.Background()
	conv, _, err := f.assistant.Ask(ctx, u, f.household, c, text)
	if err != nil {
		t.Fatalf("ask %q: %v", text, err)
	}
	var streamed strings.Builder
	m, err := f.assistant.Answer(ctx, u, conv.ID, "de", func(e domain.ChatEvent) { streamed.WriteString(e.Text) })
	if err != nil {
		t.Fatalf("answer %q: %v", text, err)
	}
	if m.Role != domain.RoleAssistant || m.Text != streamed.String() {
		t.Fatalf("answer %q: stored %+v, streamed %q", text, m, streamed.String())
	}
	return conv, m.Text
}

func TestAssistantConversationsArePrivate(t *testing.T) {
	ctx := context.Background()
	f := newAssistantFixture(t)

	c, answer := f.say(t, f.alice, nil, "  Was  kochen wir heute Abend, wenn es schnell gehen soll und wir kaum noch etwas im Haus haben?  ")
	if answer != "de: Was  kochen wir heute Abend, wenn es schnell gehen soll und wir kaum noch etwas im Haus haben?" {
		t.Fatalf("answer: %q", answer)
	}
	if c.Title != "Was kochen wir heute Abend, wenn es schnell gehen soll und…" {
		t.Fatalf("title: %q", c.Title)
	}
	if f.llm.Brief.Me != "alice" || len(f.llm.Brief.Others) != 1 || f.llm.Brief.Others[0] != "bob" || f.llm.Brief.Household != "Home" {
		t.Fatalf("brief: %+v", f.llm.Brief)
	}
	f.say(t, f.alice, &c.ID, "danke")
	if len(f.llm.History) != 3 {
		t.Fatalf("the assistant reads the conversation so far: %+v", f.llm.History)
	}
	_, ms, err := f.assistant.Conversation(ctx, f.alice, c.ID)
	if err != nil || len(ms) != 4 || ms[3].Text != "de: danke" {
		t.Fatalf("conversation: %+v, %v", ms, err)
	}

	if _, _, err := f.assistant.Conversation(ctx, f.bob, c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob reads alice's chat: want ErrNotFound, got %v", err)
	}
	if _, _, err := f.assistant.Ask(ctx, f.bob, f.household, &c.ID, "hi"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob writes in alice's chat: want ErrNotFound, got %v", err)
	}
	if _, err := f.assistant.Answer(ctx, f.bob, c.ID, "en", func(domain.ChatEvent) {}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob gets an answer in alice's chat: want ErrNotFound, got %v", err)
	}
	if err := f.assistant.DeleteConversation(ctx, f.bob, c.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob deletes alice's chat: want ErrNotFound, got %v", err)
	}
	if cs, err := f.assistant.Conversations(ctx, f.bob, f.household); err != nil || len(cs) != 0 {
		t.Fatalf("bob's list: %+v, %v", cs, err)
	}
	if _, err := f.assistant.Conversations(ctx, f.eve, f.household); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("eve isn't a member: want ErrNotFound, got %v", err)
	}
	if err := f.assistant.DeleteConversation(ctx, f.alice, c.ID); err != nil {
		t.Fatal(err)
	}
	if cs, _ := f.assistant.Conversations(ctx, f.alice, f.household); len(cs) != 0 {
		t.Fatalf("after delete: %+v", cs)
	}
}

func TestAssistantChecksBeforeStoring(t *testing.T) {
	ctx := context.Background()
	f := newAssistantFixture(t)
	off := service.NewAssistant(f.store, f.store, nil, f.entries, nil, nil, f.home, time.UTC)

	if _, _, err := off.Ask(ctx, f.alice, f.household, nil, "hi"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("without an LLM: want ErrUnavailable, got %v", err)
	}
	if _, _, err := off.Ask(ctx, f.eve, f.household, nil, "hi"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a non-member learns nothing: want ErrNotFound, got %v", err)
	}
	if _, _, err := f.assistant.Ask(ctx, f.alice, f.household, nil, strings.Repeat("x", domain.MaxChatText+1)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("too long: want ErrInvalid, got %v", err)
	}
	c, _, err := f.assistant.Ask(ctx, f.alice, f.household, nil, "down")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.assistant.Answer(ctx, f.alice, c.ID, "en", func(domain.ChatEvent) {}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("LLM down: want ErrUnavailable, got %v", err)
	}
	if cs, _ := f.assistant.Conversations(ctx, f.alice, f.household); len(cs) != 1 {
		t.Fatalf("only the checked message was stored: %+v", cs)
	}
}

func TestAssistantSeesOnlyWhatThePersonSees(t *testing.T) {
	f := newAssistantFixture(t)
	f.create(t, f.bob, domain.KindNote, domain.EntryFields{Text: "bob only", Visibility: domain.VisibilityPrivate})
	f.create(t, f.bob, domain.KindNote, domain.EntryFields{Text: "the surprise", Visibility: domain.VisibilityReveal, RevealAt: new(f.now.Add(time.Hour))})
	f.create(t, f.bob, domain.KindNote, domain.EntryFields{Text: "for both", Visibility: domain.VisibilityShared})

	_, answer := f.say(t, f.alice, nil, `/list_entries {"kind":"note"}`)
	if strings.Contains(answer, "bob only") || strings.Contains(answer, "the surprise") || !strings.Contains(answer, "for both") {
		t.Fatalf("alice's assistant reads: %s", answer)
	}
	var notes []map[string]any
	if err := json.Unmarshal([]byte(answer), &notes); err != nil || len(notes) != 2 || notes[0]["author"] != "bob" {
		t.Fatalf("notes: %s, %v", answer, err)
	}
	sealed := 0
	for _, n := range notes {
		if n["sealed"] == true {
			sealed++
		}
	}
	if sealed != 1 {
		t.Fatalf("the later note shows only that it exists: %s", answer)
	}

	if _, answer := f.say(t, f.alice, nil, `/delete_entry {}`); !strings.HasPrefix(answer, "error: ") {
		t.Fatalf("no tool deletes: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/list_entries {"kind":"note","all":true}`); !strings.HasPrefix(answer, "error: invalid input") {
		t.Fatalf("unknown fields are refused: %s", answer)
	}
}

func TestAssistantAddsEntriesAsThePerson(t *testing.T) {
	ctx := context.Background()
	f := newAssistantFixture(t)

	_, answer := f.say(t, f.alice, nil, `/add_reminder {"text":"Müll raus","visibility":"shared","due":"2026-10-29T08:00","repeat":"weekly","for":"Bob"}`)
	if !strings.Contains(answer, `"for":"bob"`) || !strings.Contains(answer, `"due":"Thu 2026-10-29 08:00"`) {
		t.Fatalf("reminder: %s", answer)
	}
	rs, _ := f.entries.List(ctx, f.bob, f.household, domain.KindReminder)
	if len(rs) != 1 || rs[0].AuthorID != f.alice.ID || *rs[0].AssigneeID != f.bob.ID || !rs[0].DueAt.Equal(time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("stored: %+v", rs)
	}

	if _, answer := f.say(t, f.alice, nil, `/add_reminder {"text":"x","visibility":"private","for":"Bob"}`); !strings.Contains(answer, "error: invalid input: assignee") {
		t.Fatalf("a private reminder can't be for someone: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/add_reminder {"text":"x","visibility":"shared","for":"Eve"}`); !strings.Contains(answer, "the members are alice, bob") {
		t.Fatalf("unknown member: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/add_event {"text":"Kino","visibility":"private","starts":"morgen"}`); !strings.Contains(answer, "YYYY-MM-DD") {
		t.Fatalf("unreadable time: %s", answer)
	}
	_, answer = f.say(t, f.alice, nil, `/add_event {"text":"Urlaub","visibility":"shared","starts":"2026-11-02","ends":"2026-11-06","all_day":true}`)
	if !strings.Contains(answer, `"starts":"Mon 2026-11-02 00:00"`) {
		t.Fatalf("event: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/add_note {"text":"Für dich","visibility":"reveal","reveal_at":"2026-12-24T18:00"}`); !strings.Contains(answer, `"visibility":"reveal"`) {
		t.Fatalf("note: %s", answer)
	}

	mine := f.create(t, f.bob, domain.KindReminder, domain.EntryFields{Text: "bob's", Visibility: domain.VisibilityPrivate})
	if _, answer := f.say(t, f.alice, nil, `/complete_reminder {"id":"`+mine.ID.String()+`"}`); answer != "error: not found" {
		t.Fatalf("alice ticks off bob's private reminder: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/complete_reminder {"id":"`+rs[0].ID.String()+`"}`); !strings.Contains(answer, `"due":"Thu 2026-11-05 08:00"`) {
		t.Fatalf("a weekly reminder moves on: %s", answer)
	}
}

func TestAssistantCooksAndSwitchesTheHome(t *testing.T) {
	f := newAssistantFixture(t)
	m := domain.Membership{Household: domain.Household{ID: f.household}}

	if _, answer := f.say(t, f.alice, nil, `/search_recipes {"query":"Curry"}`); !strings.Contains(answer, `"slug":"curry"`) || strings.Contains(answer, "pasta") {
		t.Fatalf("search: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/get_recipe {"slug":"curry"}`); !strings.Contains(answer, `"ingredients":["1 onion","salt"]`) {
		t.Fatalf("recipe: %s", answer)
	}

	_, answer := f.say(t, f.alice, nil, `/add_to_shopping_list {"items":["Kokosmilch"," Reis "],"recipe":"Green curry"}`)
	if !strings.Contains(answer, `"recipe":"Green curry"`) || !strings.Contains(answer, `"text":"Kokosmilch"`) || !strings.Contains(answer, `"text":"Reis"`) || !strings.Contains(answer, `"id":`) {
		t.Fatalf("shopping: %s", answer)
	}
	var addedItems []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(answer), &addedItems); err != nil || len(addedItems) != 2 || addedItems[0].ID == "" {
		t.Fatalf("shopping ids: %s, %v", answer, err)
	}
	if _, answer := f.say(t, f.bob, nil, `/complete_shopping_item {"id":"`+addedItems[0].ID+`"}`); !strings.Contains(answer, `"done":true`) {
		t.Fatalf("complete shopping item: %s", answer)
	}
	if _, answer := f.say(t, f.bob, nil, `/list_shopping {}`); !strings.Contains(answer, "Kokosmilch") {
		t.Fatalf("the list is shared: %s", answer)
	}

	if _, answer := f.say(t, f.alice, nil, `/plan_meal {"day":"2026-11-12","slug":"curry"}`); !strings.Contains(answer, `"recipe":"Green curry"`) {
		t.Fatalf("plan meal: %s", answer)
	}
	if _, answer := f.say(t, f.bob, nil, `/get_meal_plan {"from":"2026-11-10","days":7}`); !strings.Contains(answer, `"day":"2026-11-12"`) || !strings.Contains(answer, `"slug":"curry"`) {
		t.Fatalf("get meal plan: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/rate_meal {"slug":"curry","rating":"love"}`); !strings.Contains(answer, `"rating":"love"`) {
		t.Fatalf("rate meal: %s", answer)
	}
	if _, answer := f.say(t, f.bob, nil, `/list_meal_ratings {}`); !strings.Contains(answer, `"dish":"Green curry"`) || !strings.Contains(answer, `"rating":"love"`) {
		t.Fatalf("list meal ratings: %s", answer)
	}

	if _, answer := f.say(t, f.alice, nil, `/get_home {}`); !strings.Contains(answer, `"connected":false`) {
		t.Fatalf("not connected: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/set_light {"id":"light.kitchen","on":false}`); !strings.Contains(answer, "Home tab") {
		t.Fatalf("not connected: %s", answer)
	}
	connect(t, f.home, f.alice, m)
	if _, answer := f.say(t, f.alice, nil, `/set_light {"id":"light.kitchen","on":true}`); answer != `{"id":"light.kitchen","name":"Kitchen","on":true}` {
		t.Fatalf("already on, nothing switched: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/set_light {"id":"light.kitchen","on":false}`); answer != `{"id":"light.kitchen","name":"Kitchen","on":false}` {
		t.Fatalf("switched off: %s", answer)
	}
	if _, answer := f.say(t, f.alice, nil, `/activate_scene {"id":"scene.movie"}`); answer != `{"done":true}` || len(f.smart.Activated) != 1 {
		t.Fatalf("scene: %s, %v", answer, f.smart.Activated)
	}
	if _, answer := f.say(t, f.alice, nil, `/activate_scene {"id":"lock.front_door"}`); answer != "error: not found" {
		t.Fatalf("only scenes: %s", answer)
	}
}

func TestAssistantToolsWithoutTheChat(t *testing.T) {
	ctx := context.Background()
	f := newAssistantFixture(t)
	// Other clients bring their own model, so the tools work without one.
	assistant := service.NewAssistant(f.store, f.store, nil, f.entries, nil, nil, f.home, time.UTC)

	if len(assistant.Tools()) == 0 {
		t.Fatal("no tools")
	}
	if _, err := assistant.RunTool(ctx, f.alice, f.household, domain.ToolCall{Name: "add_reminder", Input: json.RawMessage(`{"text":"Bins out","visibility":"shared"}`)}); err != nil {
		t.Fatal(err)
	}
	out, err := assistant.RunTool(ctx, f.bob, f.household, domain.ToolCall{Name: "list_entries", Input: json.RawMessage(`{"kind":"reminder"}`)})
	if err != nil || !strings.Contains(out, "Bins out") {
		t.Fatalf("bob's list: %s, %v", out, err)
	}
	if _, err := assistant.RunTool(ctx, f.eve, f.household, domain.ToolCall{Name: "list_entries", Input: json.RawMessage(`{"kind":"reminder"}`)}); err == nil || err.Error() != "not found" {
		t.Fatalf("eve, who isn't a member: %v", err)
	}
	if _, err := assistant.RunTool(ctx, f.alice, f.household, domain.ToolCall{Name: "list_entries", Input: json.RawMessage(`{"nope":1}`)}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown field: %v", err)
	}
}

func TestAssistantAddsExpensesAndReadsTheMoney(t *testing.T) {
	ctx := context.Background()
	f := newAssistantFixture(t)
	run := func(u domain.User, name, input string) (string, error) {
		return f.assistant.RunTool(ctx, u, f.household, domain.ToolCall{Name: name, Input: json.RawMessage(input)})
	}
	if _, err := run(f.alice, "add_expense", `{"amount":12.5,"category":"groceries"}`); err == nil || err.Error() != "that part of the app isn't available right now" {
		t.Fatalf("without a budget: %v", err)
	}

	budget := service.NewBudget(f.store, f.store, time.UTC)
	f.assistant.WithBudget(budget)
	f.entries.WithBills(budget)
	out, err := run(f.alice, "add_expense", `{"amount":42.1,"category":"dining","note":"Pizza","paid_by":"`+f.bob.Name+`"}`)
	if err != nil || !strings.Contains(out, `"amount_eur":"42.10"`) || !strings.Contains(out, `"paid_by":"`+f.bob.Name+`"`) {
		t.Fatalf("add: %s, %v", out, err)
	}
	if _, err := run(f.alice, "add_expense", `{"amount":10,"category":"fun","shares":{"`+f.alice.Name+`":3}}`); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("shares off: %v", err)
	}
	if _, err := run(f.alice, "add_expense", `{"amount":10,"category":"fun","paid_by":"Mallory"}`); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown payer: %v", err)
	}
	if _, err := run(f.eve, "get_money", `{}`); err == nil || err.Error() != "not found" {
		t.Fatalf("outsider: %v", err)
	}
	out, err = run(f.alice, "get_money", `{}`)
	if err != nil || !strings.Contains(out, f.alice.Name+` (you) pays `+f.bob.Name+` 21.05 EUR`) || !strings.Contains(out, `"dining":"42.10"`) {
		t.Fatalf("money: %s, %v", out, err)
	}
	out, err = run(f.alice, "settle_up", `{"to":"`+f.bob.Name+`","amount":21.05}`)
	if err != nil || !strings.Contains(out, `"kind":"settlement"`) || !strings.Contains(out, `"amount_eur":"21.05"`) {
		t.Fatalf("settle up: %s, %v", out, err)
	}

	out, err = run(f.alice, "add_reminder", `{"text":"Rent","visibility":"shared","due":"2026-11-01T09:00","repeat":"monthly","bill":{"amount":900,"category":"home","paid_by":"`+f.alice.Name+`"}}`)
	if err != nil {
		t.Fatalf("bill: %s, %v", out, err)
	}
	es, _ := f.entries.List(ctx, f.alice, f.household, domain.KindReminder)
	if len(es) != 1 || es[0].Bill == nil || es[0].Bill.Cents != 90000 {
		t.Fatalf("bill reminder: %+v", es)
	}
}

func TestAssistantManagesChoresAndWishes(t *testing.T) {
	ctx := context.Background()
	f := newAssistantFixture(t)
	run := func(u domain.User, name, input string) (string, error) {
		return f.assistant.RunTool(ctx, u, f.household, domain.ToolCall{Name: name, Input: json.RawMessage(input)})
	}
	if _, err := run(f.alice, "add_chore", `{"title":"Bins"}`); err == nil || err.Error() != "that part of the app isn't available right now" {
		t.Fatalf("without chores: %v", err)
	}
	if _, err := run(f.alice, "add_wish", `{"title":"Espresso machine"}`); err == nil || err.Error() != "that part of the app isn't available right now" {
		t.Fatalf("without wishes: %v", err)
	}

	push := service.NewPush(f.store, &memory.Pusher{})
	moments := service.NewMoments(f.store, f.store, f.store, push, time.UTC)
	chores := service.NewChores(f.store, f.store, f.store, moments, push, time.UTC)
	wishes := service.NewWishes(f.store, f.store)
	f.assistant.WithChores(chores).WithWishes(wishes)

	out, err := run(f.alice, "add_chore", `{"title":"Bins","points":2,"repeat":"weekly","rotate":true,"for":"`+f.alice.Name+`","due":"2026-06-01"}`)
	if err != nil || !strings.Contains(out, `"title":"Bins"`) || !strings.Contains(out, `"points":2`) || !strings.Contains(out, `"for":"`+f.alice.Name+` (you)"`) {
		t.Fatalf("add chore: %s, %v", out, err)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil || created.ID == "" {
		t.Fatalf("chore id: %s, %v", out, err)
	}
	out, err = run(f.bob, "list_chores", `{}`)
	if err != nil || !strings.Contains(out, `"title":"Bins"`) || !strings.Contains(out, `"for":"`+f.alice.Name+`"`) {
		t.Fatalf("list chores: %s, %v", out, err)
	}
	out, err = run(f.alice, "complete_chore", `{"id":"`+created.ID+`"}`)
	if err != nil || !strings.Contains(out, `"title":"Bins"`) {
		t.Fatalf("complete chore: %s, %v", out, err)
	}
	var doneChore struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &doneChore); err != nil || doneChore.ID == "" {
		t.Fatalf("completion id: %s, %v", out, err)
	}
	out, err = run(f.bob, "get_chore_history", `{}`)
	if err != nil || !strings.Contains(out, `"title":"Bins"`) || !strings.Contains(out, `"points_this_week"`) {
		t.Fatalf("chore history: %s, %v", out, err)
	}
	out, err = run(f.bob, "thank_chore", `{"id":"`+doneChore.ID+`","line":"Thanks for taking the bins out!"}`)
	if err != nil || !strings.Contains(out, `"gratitude_written":true`) {
		t.Fatalf("thank chore: %s, %v", out, err)
	}
	// Rotating chore moves to Bob's turn after Alice completes it.
	out, err = run(f.bob, "list_chores", `{}`)
	if err != nil || !strings.Contains(out, `"for":"`+f.bob.Name+` (you)"`) {
		t.Fatalf("rotated chore: %s, %v", out, err)
	}

	out, err = run(f.alice, "add_wish", `{"title":"Espresso machine","note":"Chrome","url":"https://example.com/espresso","price":349.5,"hearts":3}`)
	if err != nil || !strings.Contains(out, `"title":"Espresso machine"`) || !strings.Contains(out, `"price_eur":"349.50"`) || !strings.Contains(out, `"hearts":3`) {
		t.Fatalf("add wish: %s, %v", out, err)
	}
	var wish struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &wish); err != nil || wish.ID == "" {
		t.Fatalf("wish id: %s, %v", out, err)
	}
	out, err = run(f.bob, "vote_wish", `{"id":"`+wish.ID+`","hearts":2}`)
	if err != nil || !strings.Contains(out, `"hearts":5`) {
		t.Fatalf("vote wish: %s, %v", out, err)
	}
	out, err = run(f.bob, "complete_wish", `{"id":"`+wish.ID+`"}`)
	if err != nil || !strings.Contains(out, `"got":true`) {
		t.Fatalf("complete wish: %s, %v", out, err)
	}
	out, err = run(f.bob, "list_wishes", `{}`)
	if err != nil || !strings.Contains(out, `"title":"Espresso machine"`) || !strings.Contains(out, `"note":"Chrome"`) {
		t.Fatalf("list wishes: %s, %v", out, err)
	}
	if _, err := run(f.alice, "add_wish", `{"title":"Too many hearts","hearts":5}`); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("invalid hearts: %v", err)
	}
}

func TestAssistantCoversRemainingHouseholdTools(t *testing.T) {
	ctx := context.Background()
	f := newAssistantFixture(t)
	run := func(u domain.User, name, input string) (string, error) {
		return f.assistant.RunTool(ctx, u, f.household, domain.ToolCall{Name: name, Input: json.RawMessage(input)})
	}

	// Unwired services answer that the part of the app isn't available right now.
	for _, call := range []struct{ name, input string }{
		{"list_bucket", `{}`},
		{"list_trips", `{}`},
		{"list_moments", `{}`},
		{"list_questions", `{}`},
		{"get_tonight", `{}`},
	} {
		if _, err := run(f.alice, call.name, call.input); err == nil || err.Error() != "that part of the app isn't available right now" {
			t.Fatalf("%s unwired: %v", call.name, err)
		}
	}

	push := service.NewPush(f.store, &memory.Pusher{})
	moments := service.NewMoments(f.store, f.store, f.store, push, time.UTC)
	bucket := service.NewBucket(f.store, f.store)
	trips := service.NewTrips(f.store, f.store)
	questions := service.NewQuestion(f.store, f.store, push, nil)
	gifts := service.NewGifts(f.store, f.store, f.store, push, time.UTC)
	f.assistant.WithBucket(bucket).WithTrips(trips).WithMoments(moments).WithQuestions(questions).WithGifts(gifts)

	// Household members.
	out, err := run(f.alice, "get_household", `{}`)
	if err != nil || !strings.Contains(out, `"you":"`+f.alice.Name+`"`) || !strings.Contains(out, f.bob.Name) {
		t.Fatalf("get_household: %s, %v", out, err)
	}

	// Updating and reopening entries, while sealed notes stay sealed.
	rem := f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "Call plumber", Visibility: domain.VisibilityShared})
	if out, err = run(f.alice, "update_entry", `{"id":"`+rem.ID.String()+`","text":"Call electrician"}`); err != nil || !strings.Contains(out, "Call electrician") {
		t.Fatalf("update_entry: %s, %v", out, err)
	}
	if _, err = run(f.alice, "complete_reminder", `{"id":"`+rem.ID.String()+`"}`); err != nil {
		t.Fatalf("complete_reminder: %v", err)
	}
	if out, err = run(f.alice, "reopen_reminder", `{"id":"`+rem.ID.String()+`"}`); err != nil || strings.Contains(out, `"done":true`) {
		t.Fatalf("reopen_reminder: %s, %v", out, err)
	}
	sealed := f.create(t, f.bob, domain.KindNote, domain.EntryFields{Text: "secret", Visibility: domain.VisibilityReveal, RevealAt: new(f.now.Add(24 * time.Hour))})
	if _, err = run(f.alice, "update_entry", `{"id":"`+sealed.ID.String()+`","text":"peek"}`); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("sealed note update: %v", err)
	}

	// Someday / bucket list.
	out, err = run(f.alice, "add_bucket_item", `{"kind":"watch","text":"Past Lives"}`)
	if err != nil || !strings.Contains(out, "Past Lives") {
		t.Fatalf("add_bucket_item: %s, %v", out, err)
	}
	var bucketItem struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(out), &bucketItem)
	if out, err = run(f.bob, "complete_bucket_item", `{"id":"`+bucketItem.ID+`"}`); err != nil || !strings.Contains(out, `"done":true`) {
		t.Fatalf("complete_bucket_item: %s, %v", out, err)
	}
	if out, err = run(f.bob, "list_bucket", `{"kind":"watch"}`); err != nil || !strings.Contains(out, "Past Lives") {
		t.Fatalf("list_bucket: %s, %v", out, err)
	}

	// Trips, options, votes and packing items.
	out, err = run(f.alice, "add_trip", `{"name":"Lisbon","destination":"Portugal","start":"2026-09-01","end":"2026-09-07"}`)
	if err != nil || !strings.Contains(out, `"name":"Lisbon"`) {
		t.Fatalf("add_trip: %s, %v", out, err)
	}
	var trip struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(out), &trip)
	out, err = run(f.alice, "add_trip_option", `{"trip_id":"`+trip.ID+`","title":"Alfama flat","url":"https://example.com/flat"}`)
	if err != nil || !strings.Contains(out, "Alfama flat") {
		t.Fatalf("add_trip_option: %s, %v", out, err)
	}
	var tripWithOpt struct {
		Options []struct {
			ID string `json:"id"`
		} `json:"options"`
	}
	_ = json.Unmarshal([]byte(out), &tripWithOpt)
	if out, err = run(f.bob, "vote_trip_option", `{"trip_id":"`+trip.ID+`","option_id":"`+tripWithOpt.Options[0].ID+`"}`); err != nil || !strings.Contains(out, f.bob.Name) {
		t.Fatalf("vote_trip_option: %s, %v", out, err)
	}
	out, err = run(f.alice, "add_packing_item", `{"trip_id":"`+trip.ID+`","text":"Sunscreen"}`)
	if err != nil || !strings.Contains(out, "Sunscreen") {
		t.Fatalf("add_packing_item: %s, %v", out, err)
	}
	var tripWithPack struct {
		Packing []struct {
			ID string `json:"id"`
		} `json:"packing"`
	}
	_ = json.Unmarshal([]byte(out), &tripWithPack)
	if out, err = run(f.bob, "complete_packing_item", `{"trip_id":"`+trip.ID+`","item_id":"`+tripWithPack.Packing[0].ID+`"}`); err != nil || !strings.Contains(out, `"done":true`) {
		t.Fatalf("complete_packing_item: %s, %v", out, err)
	}
	if out, err = run(f.bob, "list_trips", `{}`); err != nil || !strings.Contains(out, "Lisbon") {
		t.Fatalf("list_trips: %s, %v", out, err)
	}

	// Moments, gratitude and reactions.
	out, err = run(f.alice, "add_moment", `{"text":"Picnic by the river","visibility":"shared"}`)
	if err != nil || !strings.Contains(out, "Picnic by the river") {
		t.Fatalf("add_moment: %s, %v", out, err)
	}
	var moment struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(out), &moment)
	if out, err = run(f.bob, "react_to_moment", `{"id":"`+moment.ID+`","emoji":"❤️"}`); err != nil || !strings.Contains(out, "❤️") {
		t.Fatalf("react_to_moment: %s, %v", out, err)
	}
	if out, err = run(f.bob, "add_gratitude", `{"text":"Morning coffee together"}`); err != nil || !strings.Contains(out, `"kind":"gratitude"`) {
		t.Fatalf("add_gratitude: %s, %v", out, err)
	}
	if out, err = run(f.alice, "list_moments", `{}`); err != nil || !strings.Contains(out, "Picnic by the river") || !strings.Contains(out, "Morning coffee together") {
		t.Fatalf("list_moments: %s, %v", out, err)
	}

	// Questions stay sealed until everyone answers.
	out, err = run(f.alice, "ask_question", `{"text":"Where should we go next summer?"}`)
	if err != nil || !strings.Contains(out, "Where should we go next summer?") {
		t.Fatalf("ask_question: %s, %v", out, err)
	}
	var q struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(out), &q)
	if out, err = run(f.bob, "answer_question", `{"id":"`+q.ID+`","text":"Greece"}`); err != nil {
		t.Fatalf("bob answer_question: %s, %v", out, err)
	}
	// Alice hasn't answered yet, so Bob's answer stays sealed from her.
	if out, err = run(f.alice, "list_questions", `{}`); err != nil || strings.Contains(out, "Greece") || !strings.Contains(out, `"revealed":false`) {
		t.Fatalf("sealed question: %s, %v", out, err)
	}
	if out, err = run(f.alice, "answer_question", `{"id":"`+q.ID+`","text":"Italy"}`); err != nil || !strings.Contains(out, "Greece") || !strings.Contains(out, `"revealed":true`) {
		t.Fatalf("revealed question: %s, %v", out, err)
	}

	// Gift ideas (with id and completion) and setting own birthday.
	if out, err = run(f.alice, "set_birthday", `{"month":5,"day":14,"year":1992}`); err != nil || !strings.Contains(out, `"month":5`) {
		t.Fatalf("set_birthday: %s, %v", out, err)
	}
	out, err = run(f.alice, "add_gift_idea", `{"for":"`+f.bob.Name+`","text":"Wool scarf"}`)
	if err != nil || !strings.Contains(out, "Wool scarf") {
		t.Fatalf("add_gift_idea: %s, %v", out, err)
	}
	var idea struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(out), &idea)
	if out, err = run(f.alice, "complete_gift_idea", `{"id":"`+idea.ID+`"}`); err != nil || !strings.Contains(out, `"given":true`) {
		t.Fatalf("complete_gift_idea: %s, %v", out, err)
	}
}
