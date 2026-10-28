package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// listLimit caps how many questions one list returns.
const listLimit = 50

// notifier tells people about something that happened; *Push is the real one.
type notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, n domain.Notification)
}

type Question struct {
	households port.HouseholdRepository
	questions  port.QuestionRepository
	notify     notifier
}

func NewQuestion(households port.HouseholdRepository, questions port.QuestionRepository, notify notifier) *Question {
	return &Question{households: households, questions: questions, notify: notify}
}

func (s *Question) List(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Question, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	qs, err := s.questions.ListQuestions(ctx, householdID, listLimit)
	if err != nil {
		return nil, err
	}
	for i := range qs {
		qs[i] = sealFor(qs[i], actor.ID)
	}
	return qs, nil
}

func (s *Question) Ask(ctx context.Context, actor domain.User, householdID uuid.UUID, text string) (domain.Question, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Question{}, err
	}
	text, err := clean(text, domain.MaxQuestionText)
	if err != nil {
		return domain.Question{}, err
	}
	return s.questions.CreateQuestion(ctx, householdID, actor.ID, text)
}

func (s *Question) Answer(ctx context.Context, actor domain.User, questionID uuid.UUID, text string) (domain.Question, error) {
	q, err := s.questions.GetQuestion(ctx, questionID)
	if err != nil {
		return domain.Question{}, err
	}
	if _, err := s.households.Role(ctx, q.HouseholdID, actor.ID); err != nil {
		return domain.Question{}, err
	}
	text, err = clean(text, domain.MaxAnswerText)
	if err != nil {
		return domain.Question{}, err
	}
	q, err = s.questions.Answer(ctx, questionID, actor.ID, text)
	if err != nil {
		return domain.Question{}, err
	}
	// Answer refuses once a question is revealed, so this is the answer that revealed it.
	if q.Revealed() {
		s.notifyRevealed(ctx, actor, q)
	}
	return sealFor(q, actor.ID), nil
}

// notifyRevealed tells everyone but the last to answer that the answers are open.
func (s *Question) notifyRevealed(ctx context.Context, actor domain.User, q domain.Question) {
	members, err := s.households.Members(ctx, q.HouseholdID)
	if err != nil {
		slog.WarnContext(ctx, "question revealed: list members", "err", err)
		return
	}
	var others []uuid.UUID
	for _, m := range members {
		if m.User.ID != actor.ID {
			others = append(others, m.User.ID)
		}
	}
	s.notify.Notify(ctx, others, domain.Notification{
		Key:  "push.revealed",
		Args: []any{"name", actor.Name, "question", q.Text},
		URL:  "/us",
		Tag:  "question-" + q.ID.String(),
	})
}

// sealFor hides other members' answers from viewer until the question is revealed.
func sealFor(q domain.Question, viewer uuid.UUID) domain.Question {
	if q.Revealed() {
		return q
	}
	answers := make([]domain.Answer, len(q.Answers))
	for i, a := range q.Answers {
		if a.UserID != viewer {
			a.Text, a.Sealed = "", true
		}
		answers[i] = a
	}
	q.Answers = answers
	return q
}

func clean(text string, maxLen int) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxLen {
		return "", fmt.Errorf("%w: text must be 1-%d characters", domain.ErrInvalid, maxLen)
	}
	return text, nil
}
