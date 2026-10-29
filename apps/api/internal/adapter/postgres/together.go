package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) CreateInvite(ctx context.Context, householdID, createdBy uuid.UUID, codeHash []byte, expiresAt time.Time) error {
	return s.q.CreateInvite(ctx, sqlcgen.CreateInviteParams{CodeHash: codeHash, HouseholdID: householdID, CreatedBy: createdBy, ExpiresAt: expiresAt})
}

func (s *Store) AcceptInvite(ctx context.Context, codeHash []byte, userID uuid.UUID) (domain.Membership, error) {
	var m domain.Membership
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		// Locking the invite row makes two people racing for one link get one membership.
		householdID, err := q.LockOpenInvite(ctx, codeHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		h, err := q.GetHousehold(ctx, householdID)
		if err != nil {
			return err
		}
		role, err := q.GetMembershipRole(ctx, sqlcgen.GetMembershipRoleParams{HouseholdID: householdID, UserID: userID})
		if err == nil {
			m = domain.Membership{Household: toHousehold(h), Role: domain.Role(role)}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := q.AddMember(ctx, sqlcgen.AddMemberParams{HouseholdID: householdID, UserID: userID, Role: sqlcgen.HouseholdRoleMember}); err != nil {
			return err
		}
		if err := q.UseInvite(ctx, sqlcgen.UseInviteParams{CodeHash: codeHash, UsedBy: &userID}); err != nil {
			return err
		}
		m = domain.Membership{Household: toHousehold(h), Role: domain.RoleMember}
		return nil
	})
	return m, err
}

func (s *Store) CreateQuestion(ctx context.Context, householdID, askedBy uuid.UUID, text string) (domain.Question, error) {
	q, err := s.q.CreateQuestion(ctx, sqlcgen.CreateQuestionParams{HouseholdID: householdID, AskedBy: askedBy, Text: text})
	if err != nil {
		return domain.Question{}, err
	}
	return toQuestion(q, nil), nil
}

func (s *Store) GetQuestion(ctx context.Context, id uuid.UUID) (domain.Question, error) {
	return loadQuestion(ctx, s.q, id)
}

func (s *Store) ListQuestions(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.Question, error) {
	rows, err := s.q.ListQuestions(ctx, sqlcgen.ListQuestionsParams{HouseholdID: householdID, Limit: int32(min(limit, 1000))}) //nolint:gosec // bounded above
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	answers, err := s.q.ListAnswers(ctx, ids)
	if err != nil {
		return nil, err
	}
	byQuestion := map[uuid.UUID][]sqlcgen.Answer{}
	for _, a := range answers {
		byQuestion[a.QuestionID] = append(byQuestion[a.QuestionID], a)
	}
	out := make([]domain.Question, len(rows))
	for i, r := range rows {
		out[i] = toQuestion(r, byQuestion[r.ID])
	}
	return out, nil
}

func (s *Store) Answer(ctx context.Context, questionID, userID uuid.UUID, text string) (domain.Question, error) {
	var out domain.Question
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		// The row lock serialises answers to one question, so the last of two simultaneous
		// answers always sees the other and reveals.
		row, err := q.LockQuestion(ctx, questionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if row.RevealedAt != nil {
			return domain.ErrConflict
		}
		if err := q.UpsertAnswer(ctx, sqlcgen.UpsertAnswerParams{QuestionID: questionID, UserID: userID, Text: text}); err != nil {
			return err
		}
		p, err := q.AnswerProgress(ctx, sqlcgen.AnswerProgressParams{QuestionID: questionID, HouseholdID: row.HouseholdID})
		if err != nil {
			return err
		}
		if p.Members >= 2 && p.Answered == p.Members {
			if err := q.RevealQuestion(ctx, questionID); err != nil {
				return err
			}
		}
		out, err = loadQuestion(ctx, q, questionID)
		return err
	})
	return out, err
}

func (s *Store) RewordQuestion(ctx context.Context, questionID uuid.UUID, text string) (domain.Question, error) {
	_, err := s.q.RewordQuestion(ctx, sqlcgen.RewordQuestionParams{ID: questionID, Text: text})
	if errors.Is(err, pgx.ErrNoRows) {
		// Either it doesn't exist or someone else has answered.
		if _, err := s.q.GetQuestion(ctx, questionID); errors.Is(err, pgx.ErrNoRows) {
			return domain.Question{}, domain.ErrNotFound
		} else if err != nil {
			return domain.Question{}, err
		}
		return domain.Question{}, domain.ErrConflict
	}
	if err != nil {
		return domain.Question{}, err
	}
	return loadQuestion(ctx, s.q, questionID)
}

func (s *Store) DeleteQuestion(ctx context.Context, questionID uuid.UUID) error {
	n, err := s.q.DeleteQuestion(ctx, questionID)
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}

func loadQuestion(ctx context.Context, q *sqlcgen.Queries, id uuid.UUID) (domain.Question, error) {
	row, err := q.GetQuestion(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Question{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Question{}, err
	}
	answers, err := q.ListAnswers(ctx, []uuid.UUID{id})
	if err != nil {
		return domain.Question{}, err
	}
	return toQuestion(row, answers), nil
}

func toQuestion(q sqlcgen.Question, answers []sqlcgen.Answer) domain.Question {
	out := domain.Question{
		ID: q.ID, HouseholdID: q.HouseholdID, AskedBy: q.AskedBy, Text: q.Text,
		CreatedAt: q.CreatedAt, RevealedAt: q.RevealedAt, Answers: make([]domain.Answer, len(answers)),
	}
	for i, a := range answers {
		out.Answers[i] = domain.Answer{UserID: a.UserID, Text: a.Text, AnsweredAt: a.AnsweredAt}
	}
	return out
}
