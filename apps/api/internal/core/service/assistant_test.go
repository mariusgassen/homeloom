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
	shopping := service.NewShopping(f.store, f.store)
	cook := service.NewCook(f.store, memory.NewRecipeBook(recipes()...), nil, shopping)
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

	if _, answer := f.say(t, f.alice, nil, `/add_to_shopping_list {"items":["Kokosmilch"," Reis "],"recipe":"Green curry"}`); answer != `[{"recipe":"Green curry","text":"Kokosmilch"},{"recipe":"Green curry","text":"Reis"}]` {
		t.Fatalf("shopping: %s", answer)
	}
	if _, answer := f.say(t, f.bob, nil, `/list_shopping {}`); !strings.Contains(answer, "Kokosmilch") {
		t.Fatalf("the list is shared: %s", answer)
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
