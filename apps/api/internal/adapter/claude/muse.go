package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

const museInstructions = `You write questions for a couple to answer together in an app. Each answers privately,
and both answers open at the same time once both have answered. Good questions are short (one sentence, under
140 characters), warm, specific and open-ended, fun to answer in a few sentences, and make them curious about the
other's answer. Never anything that could embarrass, hurt or start a fight; no yes/no questions.
You get themes to write in the spirit of, one question per theme, and the questions they already asked:
never repeat or closely rephrase those.`

// Muse writes questions with Claude.
type Muse struct{ structured }

var _ port.Muse = (*Muse)(nil)

// NewMuse asks the given model with an API key; opts are for tests (e.g. option.WithBaseURL).
func NewMuse(apiKey, model string, opts ...option.RequestOption) *Muse {
	return &Muse{newStructured(apiKey, model, opts)}
}

var museSchema = object(map[string]any{
	"questions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
})

func (m *Muse) Questions(ctx context.Context, lang string, themes, recent []string, n int) ([]string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "<themes>\n%s\n</themes>\n<asked>\n", strings.Join(themes, "\n"))
	for _, q := range recent {
		b.WriteString(oneLine(q) + "\n")
	}
	fmt.Fprintf(&b, "</asked>\nWrite %d new questions in %s.", n, language(lang))

	var out struct {
		Questions []string `json:"questions"`
	}
	if err := m.ask(ctx, []anthropic.BetaTextBlockParam{{Text: museInstructions}}, b.String(), museSchema, &out); err != nil {
		return nil, err
	}
	for i, q := range out.Questions {
		out.Questions[i] = strings.TrimSpace(q)
	}
	return out.Questions, nil
}
