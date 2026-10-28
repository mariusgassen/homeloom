package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// listLimit caps how many questions one list returns.
const listLimit = 50

type Question struct {
	households port.HouseholdRepository
	questions  port.QuestionRepository
}

func NewQuestion(households port.HouseholdRepository, questions port.QuestionRepository) *Question {
	return &Question{households: households, questions: questions}
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
	return sealFor(q, actor.ID), nil
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
