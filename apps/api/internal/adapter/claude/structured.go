package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/anthropics/anthropic-sdk-go/v2/shared/constant"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// structured asks for one answer in the shape of schema and decodes it into out. Every failure is
// domain.ErrUnavailable, so callers can fall back.
type structured struct {
	client anthropic.Client
	model  string
}

func newStructured(apiKey, model string, opts []option.RequestOption) structured {
	return structured{
		client: anthropic.NewClient(append([]option.RequestOption{option.WithAPIKey(apiKey), option.WithMaxRetries(1)}, opts...)...),
		model:  model,
	}
}

func (c structured) ask(ctx context.Context, system []anthropic.BetaTextBlockParam, user string, schema map[string]any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	msg, err := c.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     c.model,
		MaxTokens: 4000,
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		System:    system,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user))},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortLow,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
	})
	if err != nil {
		return fmt.Errorf("%w: claude: %w", domain.ErrUnavailable, err)
	}
	if msg.StopReason != anthropic.BetaStopReasonEndTurn {
		return fmt.Errorf("%w: claude stopped with %s", domain.ErrUnavailable, msg.StopReason)
	}
	var text strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	if err := json.Unmarshal([]byte(text.String()), out); err != nil {
		return fmt.Errorf("%w: claude answered no JSON: %w", domain.ErrUnavailable, err)
	}
	return nil
}

// object is a JSON schema for an object whose properties are all required.
func object(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for k := range properties {
		required = append(required, k)
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
