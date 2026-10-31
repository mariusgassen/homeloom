package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

const (
	// choreHistoryDays is how far back the history goes.
	choreHistoryDays = 42
	// choreHistoryLimit caps the history; choreTallyLimit caps what one fairness tally reads.
	choreHistoryLimit = 300
	choreTallyLimit   = 2000
	// choreNudgeHour is the local hour from which the evening nudge about chores still open goes out.
	choreNudgeHour = 18
	// choreNudgeTitles is how many chores the nudge names before it says "…".
	choreNudgeTitles = 3
)

// gratitudes writes and reads a member's gratitude lines, as the Moments service does.
type gratitudes interface {
	Month(ctx context.Context, actor domain.User, householdID uuid.UUID, day time.Time) (domain.MomentMonth, error)
	Create(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.MomentKind, f domain.MomentFields) (domain.Moment, error)
}

// Chores is the household's chores: each is someone's turn or anyone's, moves to its next day
// when done and, if it rotates, to the next member. What was done is kept with its points, so
// the household sees who did how much, and can say thanks.
type Chores struct {
	households port.HouseholdRepository
	chores     port.ChoreRepository
	jobs       port.JobRepository
	gratitude  gratitudes
	notify     notifier
	loc        *time.Location
	now        func() time.Time
}

// NewChores counts days in loc, the household's time zone.
func NewChores(households port.HouseholdRepository, chores port.ChoreRepository, jobs port.JobRepository, gratitude gratitudes, notify notifier, loc *time.Location) *Chores {
	return &Chores{households: households, chores: chores, jobs: jobs, gratitude: gratitude, notify: notify, loc: loc, now: time.Now}
}

func (s *Chores) List(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Chore, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.chores.Chores(ctx, householdID, domain.MaxChores)
}

func (s *Chores) Create(ctx context.Context, actor domain.User, householdID uuid.UUID, f domain.ChoreFields) (domain.Chore, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.Chore{}, err
	}
	if f, err = s.check(actor, members, f); err != nil {
		return domain.Chore{}, err
	}
	return s.chores.CreateChore(ctx, domain.Chore{HouseholdID: householdID, ChoreFields: f, CreatedBy: &actor.ID})
}

// Update changes a chore; every member may, as chores are shared.
func (s *Chores) Update(ctx context.Context, actor domain.User, householdID, id uuid.UUID, f domain.ChoreFields) (domain.Chore, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.Chore{}, err
	}
	c, err := s.chores.GetChore(ctx, householdID, id)
	if err != nil {
		return domain.Chore{}, err
	}
	if c.ChoreFields, err = s.check(actor, members, f); err != nil {
		return domain.Chore{}, err
	}
	return s.chores.UpdateChore(ctx, c)
}

func (s *Chores) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.chores.DeleteChore(ctx, householdID, id)
}

func (s *Chores) Complete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.ChoreCompletion, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.ChoreCompletion{}, err
	}
	c, err := s.chores.GetChore(ctx, householdID, id)
	if err != nil {
		return domain.ChoreCompletion{}, err
	}
	if c.DoneAt != nil {
		return domain.ChoreCompletion{}, domain.ErrChoreDone
	}
	today := s.today()
	// Once a day is enough: a second tap would otherwise do the next time ahead of its turn.
	switch last, err := s.chores.LastChoreCompletion(ctx, householdID, id); {
	case err == nil && last.Day.Equal(today):
		return domain.ChoreCompletion{}, domain.ErrChoreDone
	case err != nil && !errors.Is(err, domain.ErrNotFound):
		return domain.ChoreCompletion{}, err
	}
	after := c
	if c.Repeat == domain.RepeatNever {
		now := s.now()
		after.DoneAt = &now
	} else {
		// Done early, it still moves past its due day, like a repeating reminder.
		after.DueOn = c.Repeat.Next(c.DueOn, later(today, c.DueOn), time.UTC)
	}
	if c.Rotate {
		next := nextMember(members, actor.ID)
		after.AssigneeID = &next
	}
	done := domain.ChoreCompletion{
		HouseholdID: householdID, ChoreID: &c.ID, Title: c.Title, Points: c.Points, UserID: actor.ID,
		Day: today, PrevDueOn: c.DueOn, PrevAssigneeID: c.AssigneeID,
	}
	return s.chores.CompleteChore(ctx, c, after, done)
}

func (s *Chores) Undo(ctx context.Context, actor domain.User, householdID, completionID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	done, err := s.chores.GetChoreCompletion(ctx, householdID, completionID)
	if err != nil {
		return err
	}
	if done.UserID != actor.ID {
		return fmt.Errorf("%w: not your completion", domain.ErrForbidden)
	}
	return s.chores.UndoChoreCompletion(ctx, done)
}

func (s *Chores) Thank(ctx context.Context, actor domain.User, householdID, completionID uuid.UUID, line string) (domain.ChoreCompletion, *domain.Moment, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.ChoreCompletion{}, nil, err
	}
	line = strings.TrimSpace(line)
	if utf8.RuneCountInString(line) > domain.MaxGratitudeText {
		return domain.ChoreCompletion{}, nil, domain.TextError{Max: domain.MaxGratitudeText}
	}
	done, err := s.chores.GetChoreCompletion(ctx, householdID, completionID)
	if err != nil {
		return domain.ChoreCompletion{}, nil, err
	}
	if done.UserID == actor.ID {
		return domain.ChoreCompletion{}, nil, domain.ErrOwnChore
	}
	added, err := s.chores.ThankChoreCompletion(ctx, householdID, completionID, actor.ID)
	if err != nil {
		return domain.ChoreCompletion{}, nil, err
	}
	if added {
		done.ThankedBy = append(done.ThankedBy, actor.ID)
		s.notify.Notify(ctx, []uuid.UUID{done.UserID}, domain.Notification{
			Key: "push.chore_thanks", Args: []any{"name", actor.Name, "chore", done.Title},
			URL: "/chores", Tag: "chore-thanks-" + done.ID.String(),
		})
	}
	if line == "" {
		return done, nil, nil
	}
	m, err := s.gratefulFor(ctx, actor, householdID, line)
	return done, m, err
}

// gratefulFor writes line as the actor's gratitude line for today, unless they wrote one already:
// a thank-you never replaces what they wrote themselves.
func (s *Chores) gratefulFor(ctx context.Context, actor domain.User, householdID uuid.UUID, line string) (*domain.Moment, error) {
	month, err := s.gratitude.Month(ctx, actor, householdID, time.Time{})
	if err != nil {
		return nil, err
	}
	today := s.today()
	if slices.ContainsFunc(month.Moments, func(m domain.Moment) bool {
		return m.Kind == domain.KindGratitude && m.AuthorID == actor.ID && m.Day.Equal(today)
	}) {
		return nil, nil
	}
	m, err := s.gratitude.Create(ctx, actor, householdID, domain.KindGratitude, domain.MomentFields{Visibility: domain.VisibilityShared, Text: line})
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Chores) History(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.ChoreCompletion, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.chores.ChoreCompletions(ctx, householdID, s.today().AddDate(0, 0, -choreHistoryDays), choreHistoryLimit)
}

func (s *Chores) Fairness(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.ChoreFairness, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.ChoreFairness{}, err
	}
	today := s.today()
	week := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	month := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	since := week
	if month.Before(week) {
		since = month
	}
	done, err := s.chores.ChoreCompletions(ctx, householdID, since, choreTallyLimit)
	if err != nil {
		return domain.ChoreFairness{}, err
	}
	return domain.ChoreFairness{Week: tally(members, done, week), Month: tally(members, done, month)}, nil
}

// tally adds up each current member's points and chores done since from.
func tally(members []domain.Member, done []domain.ChoreCompletion, from time.Time) domain.ChoreTally {
	t := domain.ChoreTally{From: from, Members: make([]domain.ChoreShare, len(members))}
	for i, m := range members {
		t.Members[i].UserID = m.User.ID
	}
	for _, d := range done {
		if d.Day.Before(from) {
			continue
		}
		if i := slices.IndexFunc(t.Members, func(sh domain.ChoreShare) bool { return sh.UserID == d.UserID }); i >= 0 {
			t.Members[i].Points += d.Points
			t.Members[i].Count++
		}
	}
	return t
}

// NotifyDaily nudges, once each evening, everyone with chores still open today: the chores that
// are their turn, and those that are anyone's. It is meant to run every minute or so; it goes
// out once per day, also with several API instances.
func (s *Chores) NotifyDaily(ctx context.Context) error {
	if s.now().In(s.loc).Hour() < choreNudgeHour {
		return nil
	}
	today := s.today()
	claimed, err := s.jobs.ClaimJobRun(ctx, "chores-due", today)
	if err != nil || !claimed {
		return err
	}
	ids, err := s.households.HouseholdIDs(ctx)
	if err != nil {
		return err
	}
	for _, hh := range ids {
		if err := s.nudge(ctx, hh, today); err != nil {
			slog.WarnContext(ctx, "chores: nudge", "err", err)
		}
	}
	return nil
}

func (s *Chores) nudge(ctx context.Context, householdID uuid.UUID, today time.Time) error {
	cs, err := s.chores.Chores(ctx, householdID, domain.MaxChores)
	if err != nil {
		return err
	}
	members, err := s.households.Members(ctx, householdID)
	if err != nil {
		return err
	}
	open := map[uuid.UUID][]string{}
	for _, c := range cs {
		if c.DueOn.After(today) {
			continue
		}
		for _, m := range members {
			if c.AssigneeID == nil || *c.AssigneeID == m.User.ID {
				open[m.User.ID] = append(open[m.User.ID], c.Title)
			}
		}
	}
	for _, m := range members {
		titles := open[m.User.ID]
		if len(titles) == 0 {
			continue
		}
		if len(titles) > choreNudgeTitles {
			titles = append(titles[:choreNudgeTitles:choreNudgeTitles], "…")
		}
		s.notify.Notify(ctx, []uuid.UUID{m.User.ID}, domain.Notification{
			Key: "push.chores_due", Args: []any{"chores", strings.Join(titles, ", ")}, URL: "/chores", Tag: "chores-due",
		})
	}
	return nil
}

// members returns the household's members, if the actor is one.
func (s *Chores) members(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Member, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.households.Members(ctx, householdID)
}

// check validates and normalises what was asked for a chore.
func (s *Chores) check(actor domain.User, members []domain.Member, f domain.ChoreFields) (domain.ChoreFields, error) {
	f.Title = strings.TrimSpace(f.Title)
	if f.Title == "" || utf8.RuneCountInString(f.Title) > domain.MaxChoreText {
		return f, domain.TextError{Max: domain.MaxChoreText}
	}
	if f.Points < 1 || f.Points > domain.MaxChorePoints {
		return f, domain.ErrChorePoints
	}
	switch f.Repeat {
	case "":
		f.Repeat = domain.RepeatNever
	case domain.RepeatNever, domain.RepeatDaily, domain.RepeatWeekly, domain.RepeatMonthly, domain.RepeatYearly:
	default:
		return f, fmt.Errorf("%w: unknown repeat %q", domain.ErrInvalid, f.Repeat)
	}
	if f.AssigneeID != nil && !slices.ContainsFunc(members, func(m domain.Member) bool { return m.User.ID == *f.AssigneeID }) {
		return f, domain.ErrChoreAssignee
	}
	// Turns start with whoever set the chore up, unless they gave it to someone.
	if f.Rotate && f.AssigneeID == nil {
		f.AssigneeID = &actor.ID
	}
	if f.DueOn.IsZero() {
		f.DueOn = s.today()
	}
	f.DueOn = time.Date(f.DueOn.Year(), f.DueOn.Month(), f.DueOn.Day(), 0, 0, 0, 0, time.UTC)
	return f, nil
}

// today is the household's date, as midnight UTC like Chore.DueOn.
func (s *Chores) today() time.Time {
	y, m, d := s.now().In(s.loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// nextMember is whose turn it is after the given member's: the next in the household's order.
func nextMember(members []domain.Member, after uuid.UUID) uuid.UUID {
	i := slices.IndexFunc(members, func(m domain.Member) bool { return m.User.ID == after })
	return members[(i+1)%len(members)].User.ID
}
