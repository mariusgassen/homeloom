package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) ListConversations(ctx context.Context, req oapi.ListConversationsRequestObject) (oapi.ListConversationsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := h.assistant.Conversations(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListConversations404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.ListConversations503JSONResponse{UnavailableJSONResponse: assistantUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListConversations200JSONResponse, len(cs))
	for i, c := range cs {
		out[i] = toConversation(c)
	}
	return out, nil
}

func (h *handlers) GetConversation(ctx context.Context, req oapi.GetConversationRequestObject) (oapi.GetConversationResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	c, ms, err := h.assistant.Conversation(ctx, u, req.ConversationId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetConversation404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetConversation503JSONResponse{UnavailableJSONResponse: assistantUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	sum := toConversation(c)
	out := oapi.GetConversation200JSONResponse{Id: sum.Id, Title: sum.Title, CreatedAt: sum.CreatedAt, UpdatedAt: sum.UpdatedAt, Messages: make([]oapi.ChatMessage, len(ms))}
	for i, m := range ms {
		out.Messages[i] = toChatMessage(m)
	}
	return out, nil
}

func (h *handlers) DeleteConversation(ctx context.Context, req oapi.DeleteConversationRequestObject) (oapi.DeleteConversationResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.assistant.DeleteConversation(ctx, u, req.ConversationId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteConversation404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.DeleteConversation503JSONResponse{UnavailableJSONResponse: assistantUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteConversation204Response{}, nil
}

func (h *handlers) Chat(ctx context.Context, req oapi.ChatRequestObject) (oapi.ChatResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	c, _, err := h.assistant.Ask(ctx, u, req.HouseholdId, req.Body.ConversationId, req.Body.Text)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.Chat404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.Chat400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxChatText))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.Chat503JSONResponse{UnavailableJSONResponse: assistantUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return chatStream{ctx: ctx, conversation: c, answer: func(ctx context.Context, emit func(domain.ChatEvent)) (domain.ChatMessage, error) {
		return h.assistant.Answer(ctx, u, c.ID, i18n.FromContext(ctx).String(), emit)
	}}, nil
}

// chatStream writes the answer as server-sent events while it is written. The answer goes on when
// the browser goes away, so it is stored for when the person comes back.
type chatStream struct {
	ctx          context.Context
	conversation domain.Conversation
	answer       func(ctx context.Context, emit func(domain.ChatEvent)) (domain.ChatMessage, error)
}

func (s chatStream) VisitChatResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	send := func(event string, data any) {
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		// A browser that went away is not an error; the answer is stored regardless.
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err == nil {
			_ = rc.Flush()
		}
	}

	send("conversation", toConversation(s.conversation))
	m, err := s.answer(context.WithoutCancel(s.ctx), func(e domain.ChatEvent) {
		switch {
		case e.Tool != "":
			send("tool", chatTool{Name: e.Tool})
		case e.Text != "":
			send("text", chatText{Text: e.Text})
		}
	})
	if errors.Is(err, domain.ErrUnavailable) {
		send("error", assistantUnavailable(s.ctx, err))
		return nil
	}
	if err != nil {
		slog.ErrorContext(s.ctx, "assistant answer failed", "err", err)
		send("error", errorBody(s.ctx, "internal", ""))
		return nil
	}
	send("done", toChatMessage(m))
	return nil
}

// chatTool and chatText are the contract's ChatTool and ChatText, which no operation names, so
// they aren't generated.
type chatTool struct {
	Name string `json:"name"`
}

type chatText struct {
	Text string `json:"text"`
}

func toConversation(c domain.Conversation) oapi.Conversation {
	return oapi.Conversation{Id: c.ID, Title: c.Title, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

func toChatMessage(m domain.ChatMessage) oapi.ChatMessage {
	return oapi.ChatMessage{Id: m.ID, Role: oapi.ChatMessageRole(m.Role), Text: m.Text, CreatedAt: m.CreatedAt}
}

// assistantUnavailable answers when no LLM is set up or it is down; the cause is logged, not sent.
func assistantUnavailable(ctx context.Context, err error) oapi.UnavailableJSONResponse {
	slog.WarnContext(ctx, "assistant unavailable", "err", err)
	return oapi.UnavailableJSONResponse(errorBody(ctx, "assistant_unavailable", ""))
}
