package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) CreateInvite(_ context.Context, householdID, _ uuid.UUID, codeHash []byte, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invites[string(codeHash)] = &invite{householdID: householdID, expiresAt: expiresAt}
	return nil
}

func (s *Store) AcceptInvite(_ context.Context, codeHash []byte, userID uuid.UUID) (domain.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.invites[string(codeHash)]
	if !ok || inv.used || time.Now().After(inv.expiresAt) {
		return domain.Membership{}, domain.ErrNotFound
	}
	h := s.households[inv.householdID]
	for _, m := range s.memberships {
		if m.householdID == inv.householdID && m.userID == userID {
			return domain.Membership{Household: h, Role: m.role}, nil
		}
	}
	s.memberships = append(s.memberships, membership{inv.householdID, userID, domain.RoleMember})
	inv.used = true
	return domain.Membership{Household: h, Role: domain.RoleMember}, nil
}

func (s *Store) CreateQuestion(_ context.Context, householdID, askedBy uuid.UUID, text string) (domain.Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := &domain.Question{ID: uuid.New(), HouseholdID: householdID, AskedBy: askedBy, Text: text, CreatedAt: time.Now()}
	s.questions = append(s.questions, q)
	return copyQuestion(q), nil
}

func (s *Store) GetQuestion(_ context.Context, id uuid.UUID) (domain.Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.findQuestion(id)
	if q == nil {
		return domain.Question{}, domain.ErrNotFound
	}
	return copyQuestion(q), nil
}

func (s *Store) ListQuestions(_ context.Context, householdID uuid.UUID, limit int) ([]domain.Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Question{}
	for _, q := range slices.Backward(s.questions) {
		if q.HouseholdID == householdID && len(out) < limit {
			out = append(out, copyQuestion(q))
		}
	}
	return out, nil
}

func (s *Store) Answer(_ context.Context, questionID, userID uuid.UUID, text string) (domain.Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.findQuestion(questionID)
	switch {
	case q == nil:
		return domain.Question{}, domain.ErrNotFound
	case q.Revealed():
		return domain.Question{}, domain.ErrConflict
	}
	q.Answers = slices.DeleteFunc(q.Answers, func(a domain.Answer) bool { return a.UserID == userID })
	q.Answers = append(q.Answers, domain.Answer{UserID: userID, Text: text, AnsweredAt: time.Now()})
	members, answered := 0, 0
	for _, m := range s.memberships {
		if m.householdID != q.HouseholdID {
			continue
		}
		members++
		if slices.ContainsFunc(q.Answers, func(a domain.Answer) bool { return a.UserID == m.userID }) {
			answered++
		}
	}
	if members >= 2 && answered == members {
		now := time.Now()
		q.RevealedAt = &now
	}
	return copyQuestion(q), nil
}

func (s *Store) findQuestion(id uuid.UUID) *domain.Question {
	for _, q := range s.questions {
		if q.ID == id {
			return q
		}
	}
	return nil
}

func copyQuestion(q *domain.Question) domain.Question {
	c := *q
	c.Answers = slices.Clone(q.Answers)
	return c
}
