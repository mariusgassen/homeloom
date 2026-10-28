package claude_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/claude"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// streamAPI answers the n-th request with the n-th list of server-sent events and keeps the requests.
func streamAPI(t *testing.T, rounds ...[]string) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var seen []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		seen = append(seen, req)
		if len(seen) > len(rounds) {
			http.Error(w, `{"type":"error","error":{"type":"api_error","message":"no more"}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range rounds[len(seen)-1] {
			var head struct{ Type string }
			_ = json.Unmarshal([]byte(ev), &head)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", head.Type, ev)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func message(stop string, blocks ...string) []string {
	evs := []string{`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}`}
	evs = append(evs, blocks...)
	return append(evs,
		`{"type":"message_delta","delta":{"stop_reason":"`+stop+`","stop_sequence":null},"usage":{"output_tokens":5}}`,
		`{"type":"message_stop"}`)
}

func textBlock(i int, parts ...string) []string {
	out := []string{fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"text","text":""}}`, i)}
	for _, p := range parts {
		b, _ := json.Marshal(p)
		out = append(out, fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":%s}}`, i, b))
	}
	return append(out, fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, i))
}

func toolBlock(i int, id, name, input string) []string {
	b, _ := json.Marshal(input)
	return []string{
		fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":"%s","name":"%s","input":{}}}`, i, id, name),
		fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`, i, b),
		fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, i),
	}
}

var brief = domain.AssistantBrief{Lang: "de", Now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC), Location: time.UTC, Me: "Alice", Household: "Home", Others: []string{"Bob"}}

var tools = []domain.Tool{{Name: "get_home", Description: "Read the home.", Schema: map[string]any{
	"type": "object", "properties": map[string]any{"room": map[string]any{"type": "string"}}, "required": []string{"room"}, "additionalProperties": false,
}}}

func TestReplyRunsToolsAndStreams(t *testing.T) {
	srv, seen := streamAPI(t,
		message("tool_use", append(textBlock(0, "Ich ", "schaue."), toolBlock(1, "tu_1", "get_home", `{"room":"Küche"}`)...)...),
		message("end_turn", textBlock(0, "Das Licht ist an.")...),
	)
	a := claude.NewAssistant("key", "claude-opus-5-5", option.WithBaseURL(srv.URL))

	var calls []domain.ToolCall
	var events []domain.ChatEvent
	history := []domain.ChatMessage{
		{Role: domain.RoleAssistant, Text: "stray"},
		{Role: domain.RoleUser, Text: "Hallo"}, {Role: domain.RoleUser, Text: "Ist das Licht an?"},
	}
	answer, err := a.Reply(context.Background(), brief, history, tools,
		func(_ context.Context, c domain.ToolCall) (string, error) {
			calls = append(calls, c)
			return `{"on":true}`, nil
		},
		func(e domain.ChatEvent) { events = append(events, e) })
	if err != nil || answer != "Ich schaue.\n\nDas Licht ist an." {
		t.Fatalf("answer: %q, %v", answer, err)
	}
	if len(calls) != 1 || calls[0].Name != "get_home" || string(calls[0].Input) != `{"room":"Küche"}` {
		t.Fatalf("calls: %+v", calls)
	}
	want := []domain.ChatEvent{{Text: "Ich "}, {Text: "schaue."}, {Tool: "get_home"}, {Text: "Das Licht ist an."}}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Fatalf("events: %+v", events)
	}

	first := (*seen)[0]
	msgs, _ := json.Marshal(first["messages"])
	if strings.Contains(string(msgs), "stray") || !strings.Contains(string(msgs), `Hallo\n\nIst das Licht an?`) {
		t.Errorf("history not joined into one user turn: %s", msgs)
	}
	system, _ := json.Marshal(first["system"])
	if !strings.Contains(string(system), "Wednesday, 2026-10-28 12:00") || !strings.Contains(string(system), "German") || !strings.Contains(string(system), "Bob") {
		t.Errorf("situation missing: %s", system)
	}
	toolsSent, _ := json.Marshal(first["tools"])
	if !strings.Contains(string(toolsSent), `"additionalProperties":false`) || !strings.Contains(string(toolsSent), `"required":["room"]`) {
		t.Errorf("tool schema: %s", toolsSent)
	}
	if first["fallbacks"] != "default" {
		t.Errorf("fallbacks: %v", first["fallbacks"])
	}
	second, _ := json.Marshal((*seen)[1]["messages"])
	if !strings.Contains(string(second), `"tool_use_id":"tu_1"`) || !strings.Contains(string(second), `"type":"tool_use"`) {
		t.Errorf("tool result not sent back after the model's turn: %s", second)
	}
}

func TestReplyReportsToolErrorsToTheModel(t *testing.T) {
	srv, seen := streamAPI(t,
		message("tool_use", toolBlock(0, "tu_1", "get_home", `{"room":"Bad"}`)...),
		message("end_turn", textBlock(0, "Geht gerade nicht.")...),
	)
	a := claude.NewAssistant("key", "claude-opus-5-5", option.WithBaseURL(srv.URL))
	answer, err := a.Reply(context.Background(), brief, []domain.ChatMessage{{Role: domain.RoleUser, Text: "Licht?"}}, tools,
		func(context.Context, domain.ToolCall) (string, error) { return "", errors.New("not connected") },
		func(domain.ChatEvent) {})
	if err != nil || answer != "Geht gerade nicht." {
		t.Fatalf("answer: %q, %v", answer, err)
	}
	second, _ := json.Marshal((*seen)[1]["messages"])
	if !strings.Contains(string(second), `"is_error":true`) || !strings.Contains(string(second), "not connected") {
		t.Errorf("tool error not sent: %s", second)
	}
}

func TestReplyIsUnavailableWhenClaudeFails(t *testing.T) {
	refused, _ := streamAPI(t, message("refusal"))
	down, _ := streamAPI(t)
	for name, url := range map[string]string{"refusal": refused.URL, "error": down.URL} {
		a := claude.NewAssistant("key", "claude-opus-5-5", option.WithBaseURL(url))
		_, err := a.Reply(context.Background(), brief, []domain.ChatMessage{{Role: domain.RoleUser, Text: "Hallo"}}, tools,
			func(context.Context, domain.ToolCall) (string, error) { return "", nil }, func(domain.ChatEvent) {})
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("%s: want ErrUnavailable, got %v", name, err)
		}
	}
}
