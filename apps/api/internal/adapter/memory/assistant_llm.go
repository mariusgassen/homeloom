package memory

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Assistant is a stand-in for the LLM. For the message "/<tool> <json input>" it calls that tool
// and answers with its result, or "error: <message>"; for "down" it is unavailable; anything else
// it answers with "<lang>: <message>". It keeps what it was last given in Brief and History.
type Assistant struct {
	Brief   domain.AssistantBrief
	History []domain.ChatMessage
}

var _ port.Assistant = (*Assistant)(nil)

func (a *Assistant) Reply(ctx context.Context, brief domain.AssistantBrief, history []domain.ChatMessage, _ []domain.Tool, run port.ToolRunner, emit func(domain.ChatEvent)) (string, error) {
	a.Brief, a.History = brief, history
	last := history[len(history)-1].Text
	if last == "down" {
		return "", domain.ErrUnavailable
	}
	answer := brief.Lang + ": " + last
	if rest, ok := strings.CutPrefix(last, "/"); ok {
		name, input, _ := strings.Cut(rest, " ")
		emit(domain.ChatEvent{Tool: name})
		out, err := run(ctx, domain.ToolCall{Name: name, Input: json.RawMessage(input)})
		answer = out
		if err != nil {
			answer = "error: " + err.Error()
		}
	}
	emit(domain.ChatEvent{Text: answer})
	return answer, nil
}
