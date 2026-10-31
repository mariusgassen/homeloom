package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

const (
	// conversationLimit is how many conversations a person sees in the list.
	conversationLimit = 30
	// historyLimit is how many earlier messages of a conversation the assistant reads.
	historyLimit = 40
	// answerTimeout bounds one answer, every model call and tool included.
	answerTimeout = 2 * time.Minute
)

// Assistant is the chat assistant. It answers as and for the person who asks: its tools call the
// same services the app does, with that person as the actor, so it sees and changes only what
// they could themselves. Without an LLM (llm is nil) the assistant is unavailable.
type Assistant struct {
	households    port.HouseholdRepository
	conversations port.ConversationRepository
	llm           port.Assistant
	entries       port.EntryService
	cook          port.CookService
	shopping      port.ShoppingService
	home          port.HomeService
	// budget is nil when the money tools aren't wired; they then answer that it isn't available.
	budget port.BudgetService
	loc    *time.Location
	now    func() time.Time
}

func NewAssistant(households port.HouseholdRepository, conversations port.ConversationRepository, llm port.Assistant,
	entries port.EntryService, cook port.CookService, shopping port.ShoppingService, home port.HomeService, loc *time.Location,
) *Assistant {
	return &Assistant{
		households: households, conversations: conversations, llm: llm,
		entries: entries, cook: cook, shopping: shopping, home: home, loc: loc, now: time.Now,
	}
}

// WithBudget gives the assistant the household's money: adding expenses and reading the balance.
func (s *Assistant) WithBudget(budget port.BudgetService) *Assistant {
	s.budget = budget
	return s
}

func (s *Assistant) Conversations(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Conversation, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return nil, err
	}
	return s.conversations.ListConversations(ctx, householdID, actor.ID, conversationLimit)
}

func (s *Assistant) Conversation(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Conversation, []domain.ChatMessage, error) {
	c, err := s.own(ctx, actor, id)
	if err != nil {
		return domain.Conversation{}, nil, err
	}
	ms, err := s.conversations.ChatMessages(ctx, id, historyLimit)
	if err != nil {
		return domain.Conversation{}, nil, err
	}
	return c, ms, nil
}

func (s *Assistant) DeleteConversation(ctx context.Context, actor domain.User, id uuid.UUID) error {
	if _, err := s.own(ctx, actor, id); err != nil {
		return err
	}
	return s.conversations.DeleteConversation(ctx, id)
}

func (s *Assistant) Ask(ctx context.Context, actor domain.User, householdID uuid.UUID, conversationID *uuid.UUID, text string) (domain.Conversation, domain.ChatMessage, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Conversation{}, domain.ChatMessage{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > domain.MaxChatText {
		return domain.Conversation{}, domain.ChatMessage{}, domain.TextError{Max: domain.MaxChatText}
	}
	var c domain.Conversation
	var err error
	if conversationID != nil {
		if c, err = s.own(ctx, actor, *conversationID); err != nil {
			return domain.Conversation{}, domain.ChatMessage{}, err
		}
		if c.HouseholdID != householdID {
			return domain.Conversation{}, domain.ChatMessage{}, domain.ErrNotFound
		}
	} else if c, err = s.conversations.CreateConversation(ctx, domain.Conversation{HouseholdID: householdID, UserID: actor.ID, Title: title(text)}); err != nil {
		return domain.Conversation{}, domain.ChatMessage{}, err
	}
	m, err := s.conversations.AddChatMessage(ctx, domain.ChatMessage{ConversationID: c.ID, Role: domain.RoleUser, Text: text})
	if err != nil {
		return domain.Conversation{}, domain.ChatMessage{}, err
	}
	return c, m, nil
}

func (s *Assistant) Answer(ctx context.Context, actor domain.User, conversationID uuid.UUID, lang string, emit func(domain.ChatEvent)) (domain.ChatMessage, error) {
	c, err := s.own(ctx, actor, conversationID)
	if err != nil {
		return domain.ChatMessage{}, err
	}
	history, err := s.conversations.ChatMessages(ctx, c.ID, historyLimit)
	if err != nil {
		return domain.ChatMessage{}, err
	}
	if len(history) == 0 || history[len(history)-1].Role != domain.RoleUser {
		return domain.ChatMessage{}, fmt.Errorf("%w: nothing to answer", domain.ErrConflict)
	}
	h, err := s.households.Get(ctx, c.HouseholdID)
	if err != nil {
		return domain.ChatMessage{}, err
	}
	members, err := s.households.Members(ctx, c.HouseholdID)
	if err != nil {
		return domain.ChatMessage{}, err
	}
	brief := domain.AssistantBrief{Lang: lang, Now: s.now(), Location: s.loc, Me: actor.Name, Household: h.Name}
	for _, m := range members {
		if m.User.ID != actor.ID {
			brief.Others = append(brief.Others, m.User.Name)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, answerTimeout)
	defer cancel()
	box := &toolbox{s: s, actor: actor, householdID: c.HouseholdID, members: members}
	text, err := s.llm.Reply(ctx, brief, history, tools, box.run, emit)
	if err != nil {
		return domain.ChatMessage{}, err
	}
	// The reply is stored even when the person has gone, so it is there when they come back.
	return s.conversations.AddChatMessage(context.WithoutCancel(ctx), domain.ChatMessage{ConversationID: c.ID, Role: domain.RoleAssistant, Text: text})
}

// Tools are what the assistant can do; the MCP server offers the same.
func (s *Assistant) Tools() []domain.Tool {
	return slices.Clone(tools)
}

// RunTool runs a tool for a client other than the assistant's own chat. It needs no LLM.
func (s *Assistant) RunTool(ctx context.Context, actor domain.User, householdID uuid.UUID, call domain.ToolCall) (string, error) {
	box := &toolbox{s: s, actor: actor, householdID: householdID}
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return "", box.explain(ctx, call.Name, err)
	}
	members, err := s.households.Members(ctx, householdID)
	if err != nil {
		return "", box.explain(ctx, call.Name, err)
	}
	box.members = members
	return box.run(ctx, call)
}

// own returns the actor's conversation; anyone else's is domain.ErrNotFound.
func (s *Assistant) own(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Conversation, error) {
	c, err := s.conversations.GetConversation(ctx, id)
	if err != nil {
		return domain.Conversation{}, err
	}
	if c.UserID != actor.ID {
		return domain.Conversation{}, domain.ErrNotFound
	}
	// A member who left keeps no access to the household through old conversations.
	if err := s.ready(ctx, actor, c.HouseholdID); err != nil {
		return domain.Conversation{}, err
	}
	return c, nil
}

// ready checks membership first, so non-members learn nothing about whether the assistant is set up.
func (s *Assistant) ready(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	if s.llm == nil {
		return fmt.Errorf("%w: no LLM is configured", domain.ErrUnavailable)
	}
	return nil
}

func title(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= domain.MaxChatTitle {
		return text
	}
	return strings.TrimSpace(string([]rune(text)[:domain.MaxChatTitle-1])) + "…"
}

// errNoAnswer is what the model is told about a failure it can't do anything about.
var errNoAnswer = errors.New("that didn't work just now; tell the person and suggest trying again later")
