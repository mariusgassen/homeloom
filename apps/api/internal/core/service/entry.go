package service

import (
	"bytes"
	"cmp"
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
	// entryListLimit caps how many entries one list returns.
	entryListLimit = 200
	// DueBatch is how many due entries one round of NotifyDue claims at most.
	DueBatch = 100
	// feedLimit caps how many entries a calendar feed holds; feedHistory is how far back it goes.
	feedLimit   = 1000
	feedHistory = 90 * 24 * time.Hour
)

type Entry struct {
	households port.HouseholdRepository
	entries    port.EntryRepository
	feeds      port.CalendarFeedRepository
	notify     notifier
	loc        *time.Location
	now        func() time.Time
	// photos is where notes' photos go; nil keeps them in the entry repository.
	photos port.PhotoStore
	// bills books a bill when its reminder is ticked off; nil means reminders can't be bills.
	bills billBooker
}

// billBooker is the part of the budget a bill needs.
type billBooker interface {
	Add(ctx context.Context, actor domain.User, householdID uuid.UUID, f domain.ExpenseFields) (domain.Expense, error)
}

// NewEntry places all-day events and repeats in loc, the household's time zone, so they keep
// their wall-clock time.
func NewEntry(households port.HouseholdRepository, entries port.EntryRepository, feeds port.CalendarFeedRepository, notify notifier, loc *time.Location) *Entry {
	return &Entry{households: households, entries: entries, feeds: feeds, notify: notify, loc: loc, now: time.Now}
}

// WithPhotos keeps new photos in notes in photos; the entry repository then only keeps that they
// exist. Photos from before stay where they are and are still served from there.
func (s *Entry) WithPhotos(photos port.PhotoStore) *Entry {
	s.photos = photos
	return s
}

// WithBills lets shared reminders be bills that book an expense in budget when ticked off.
func (s *Entry) WithBills(budget billBooker) *Entry {
	s.bills = budget
	return s
}

func (s *Entry) List(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.EntryKind) ([]domain.Entry, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	now := s.now()
	y, m, d := now.In(s.loc).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, s.loc)
	es, err := s.entries.ListEntries(ctx, householdID, actor.ID, kind, today, entryListLimit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Entry, 0, len(es))
	for _, e := range es {
		if !visible(e, actor.ID) {
			continue
		}
		if e.Kind == domain.KindEvent && e.Repeat != domain.RepeatNever {
			e = s.occurrence(e, today)
		}
		out = append(out, sealEntry(e, actor.ID, now))
	}
	// Moving repeating events to their next time can change the order of the events.
	slices.SortStableFunc(out, func(a, b domain.Entry) int {
		if c := cmp.Compare(a.Kind, b.Kind); c != 0 || a.Kind != domain.KindEvent {
			return c
		}
		return a.StartsAt.Compare(*b.StartsAt)
	})
	return out, nil
}

// occurrence moves a repeating event to its first time that ends at or after since.
func (s *Entry) occurrence(e domain.Entry, since time.Time) domain.Entry {
	var length time.Duration
	if e.EndsAt != nil {
		length = e.EndsAt.Sub(*e.StartsAt)
	}
	if !e.StartsAt.Add(length).Before(since) {
		return e
	}
	start := e.Repeat.Next(*e.StartsAt, since.Add(-length-time.Nanosecond), s.loc)
	e.StartsAt = &start
	if e.EndsAt != nil {
		end := start.Add(length)
		e.EndsAt = &end
	}
	return e
}

func (s *Entry) Create(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.EntryKind, f domain.EntryFields) (domain.Entry, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Entry{}, err
	}
	f, err := s.check(ctx, householdID, kind, f)
	if err != nil {
		return domain.Entry{}, err
	}
	e := domain.Entry{HouseholdID: householdID, AuthorID: actor.ID, Kind: kind, EntryFields: f}
	s.schedule(&e)
	return s.entries.CreateEntry(ctx, e)
}

func (s *Entry) Get(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Entry, error) {
	e, err := s.load(ctx, actor, id)
	if err != nil {
		return domain.Entry{}, err
	}
	return sealEntry(e, actor.ID, s.now()), nil
}

func (s *Entry) Suggestions(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.TitleSuggestion, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.entries.ReminderSuggestions(ctx, householdID, actor.ID, domain.MaxSuggestions)
}

// Update changes an entry. With base, only if the entry is still the version last changed then,
// so an edit made offline doesn't overwrite a newer one.
func (s *Entry) Update(ctx context.Context, actor domain.User, id uuid.UUID, f domain.EntryFields, base *time.Time) (domain.Entry, error) {
	e, err := s.editable(ctx, actor, id)
	if err != nil {
		return domain.Entry{}, err
	}
	if base != nil && !base.Equal(e.UpdatedAt) {
		return domain.Entry{}, domain.ErrEntryChanged
	}
	if f.Visibility != e.Visibility && actor.ID != e.AuthorID {
		return domain.Entry{}, fmt.Errorf("%w: only the author decides who sees it", domain.ErrForbidden)
	}
	if f, err = s.check(ctx, e.HouseholdID, e.Kind, f); err != nil {
		return domain.Entry{}, err
	}
	due := e.DueAt
	e.EntryFields = f
	if e.Kind == domain.KindEvent {
		e.DueAt = s.nextAlert(e.EntryFields, s.now())
	}
	if !sameTime(e.DueAt, due) {
		s.schedule(&e)
	}
	if e, err = s.entries.UpdateEntry(ctx, e); err != nil || e.Kind != domain.KindNote {
		return e, err
	}
	return e, s.prunePhotos(ctx, e)
}

// prunePhotos deletes the note's photos its text no longer links.
func (s *Entry) prunePhotos(ctx context.Context, e domain.Entry) error {
	ids, err := s.entries.EntryPhotoIDs(ctx, e.ID)
	if err != nil {
		return err
	}
	ids = slices.DeleteFunc(ids, func(id uuid.UUID) bool { return strings.Contains(e.Text, id.String()) })
	return s.deletePhotos(ctx, e.ID, ids)
}

func (s *Entry) deletePhotos(ctx context.Context, entryID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if s.photos != nil {
		for _, id := range ids {
			if err := s.photos.DeletePhoto(ctx, notePhotoKey(entryID, id)); err != nil {
				return err
			}
		}
	}
	return s.entries.DeleteEntryPhotos(ctx, entryID, ids)
}

// notePhotoKey is where a photo in a note is kept in the PhotoStore.
func notePhotoKey(entryID, photoID uuid.UUID) string {
	return "notes/" + entryID.String() + "/" + photoID.String() + ".jpg"
}

func (s *Entry) AddPhoto(ctx context.Context, actor domain.User, id uuid.UUID, p domain.Photo) (uuid.UUID, error) {
	e, err := s.editable(ctx, actor, id)
	if err != nil {
		return uuid.Nil, err
	}
	// The app sends JPEGs; checking the bytes as well keeps anything else from being served as one.
	if e.Kind != domain.KindNote || p.ContentType != "image/jpeg" || len(p.Data) > domain.MaxPhotoBytes ||
		!bytes.HasPrefix(p.Data, []byte{0xFF, 0xD8, 0xFF}) {
		return uuid.Nil, domain.ErrNotePhoto
	}
	ids, err := s.entries.EntryPhotoIDs(ctx, id)
	if err != nil {
		return uuid.Nil, err
	}
	if len(ids) >= domain.MaxNotePhotos {
		return uuid.Nil, domain.ErrNotePhotos
	}
	if s.photos == nil {
		return s.entries.AddEntryPhoto(ctx, id, p)
	}
	// The repository keeps that the photo exists, without its bytes, so it can be counted and pruned.
	photoID, err := s.entries.AddEntryPhoto(ctx, id, domain.Photo{ContentType: p.ContentType})
	if err != nil {
		return uuid.Nil, err
	}
	if err := s.photos.PutPhoto(ctx, notePhotoKey(id, photoID), p); err != nil {
		return uuid.Nil, errors.Join(err, s.entries.DeleteEntryPhotos(ctx, id, []uuid.UUID{photoID}))
	}
	return photoID, nil
}

func (s *Entry) Photo(ctx context.Context, actor domain.User, id, photoID uuid.UUID) (domain.Photo, error) {
	e, err := s.Get(ctx, actor, id)
	if err != nil {
		return domain.Photo{}, err
	}
	if e.Sealed {
		return domain.Photo{}, domain.ErrNotFound
	}
	p, err := s.entries.EntryPhoto(ctx, id, photoID)
	if err != nil || len(p.Data) > 0 || s.photos == nil {
		return p, err
	}
	return s.photos.GetPhoto(ctx, notePhotoKey(id, photoID))
}

func (s *Entry) Delete(ctx context.Context, actor domain.User, id uuid.UUID) error {
	e, err := s.editable(ctx, actor, id)
	if err != nil {
		return err
	}
	if e.Kind == domain.KindNote {
		ids, err := s.entries.EntryPhotoIDs(ctx, id)
		if err != nil {
			return err
		}
		if err := s.deletePhotos(ctx, id, ids); err != nil {
			return err
		}
	}
	return s.entries.DeleteEntry(ctx, id)
}

func (s *Entry) SetDone(ctx context.Context, actor domain.User, id uuid.UUID, done bool) (domain.Entry, error) {
	e, err := s.load(ctx, actor, id)
	if err != nil {
		return domain.Entry{}, err
	}
	if e.Kind != domain.KindReminder {
		return domain.Entry{}, fmt.Errorf("%w: only reminders can be done", domain.ErrConflict)
	}
	now := s.now()
	if done && e.DoneAt == nil && e.Bill != nil && s.bills != nil {
		// The expense comes first: a bill that couldn't be booked stays open to tick again.
		payer := actor.ID
		if e.Bill.PaidBy != nil {
			payer = *e.Bill.PaidBy
		}
		day := now.In(s.loc)
		if _, err := s.bills.Add(ctx, actor, e.HouseholdID, domain.ExpenseFields{
			Cents: e.Bill.Cents, Category: e.Bill.Category, Note: e.Text, PaidBy: payer,
			Day: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
		}); err != nil {
			return domain.Entry{}, err
		}
	}
	switch {
	case !done:
		e.DoneAt, e.DoneBy = nil, nil
	case e.Repeat != domain.RepeatNever && e.DueAt != nil:
		// A repeating reminder is never done; it moves on to its next time, also when it is
		// ticked off early.
		next := e.Repeat.Next(*e.DueAt, later(now, *e.DueAt), s.loc)
		e.DueAt = &next
	default:
		e.DoneAt, e.DoneBy = &now, &actor.ID
	}
	return s.entries.UpdateEntry(ctx, e)
}

// NotifyDue tells the people each due reminder and event alert is for, and returns how many
// entries it handled. It is meant to run every few seconds; each due time is notified once.
func (s *Entry) NotifyDue(ctx context.Context) (int, error) {
	now := s.now()
	due, err := s.entries.ClaimDue(ctx, now, DueBatch)
	if err != nil {
		return 0, err
	}
	for _, e := range due {
		to, err := s.recipients(ctx, e)
		if err != nil {
			slog.WarnContext(ctx, "entries: find recipients", "err", err)
			continue
		}
		s.notify.Notify(ctx, to, s.notification(e))
		if e.Kind == domain.KindEvent && e.Repeat != domain.RepeatNever {
			// The alert for the next time goes out once its own due time has come.
			e.DueAt = s.nextAlert(e.EntryFields, now)
			if _, err := s.entries.UpdateEntry(ctx, e); err != nil {
				slog.WarnContext(ctx, "entries: schedule next alert", "err", err)
			}
		}
	}
	return len(due), nil
}

func (s *Entry) notification(e domain.Entry) domain.Notification {
	n := domain.Notification{
		Key:  "push.reminder",
		Args: []any{"text", e.Text},
		URL:  "/lists?entry=" + e.ID.String(),
		Tag:  string(e.Kind) + "-" + e.ID.String(),
	}
	if e.Kind != domain.KindEvent || e.DueAt == nil || e.AlertMinutes == nil {
		return n
	}
	alert := e.DueAt.In(s.loc)
	start := alert.Add(time.Duration(*e.AlertMinutes) * time.Minute)
	date, clock := start.Format("02.01."), start.Format("15:04")
	switch {
	case e.AllDay:
		n.Key = "push.event_allday"
	case alert.YearDay() == start.YearDay() && alert.Year() == start.Year():
		n.Key = "push.event"
	default:
		n.Key = "push.event_later"
	}
	n.Args = append(n.Args, "date", date, "time", clock)
	return n
}

// recipients are who an entry is for: its author when private, else its assignee or everyone.
func (s *Entry) recipients(ctx context.Context, e domain.Entry) ([]uuid.UUID, error) {
	switch {
	case e.Visibility == domain.VisibilityPrivate:
		return []uuid.UUID{e.AuthorID}, nil
	case e.AssigneeID != nil:
		return []uuid.UUID{*e.AssigneeID}, nil
	}
	members, err := s.households.Members(ctx, e.HouseholdID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(members))
	for i, m := range members {
		ids[i] = m.User.ID
	}
	return ids, nil
}

// load returns an entry the actor may see, unsealed; anything else is domain.ErrNotFound.
func (s *Entry) load(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Entry, error) {
	e, err := s.entries.GetEntry(ctx, id)
	if err != nil {
		return domain.Entry{}, err
	}
	if _, err := s.households.Role(ctx, e.HouseholdID, actor.ID); err != nil {
		return domain.Entry{}, err
	}
	if !visible(e, actor.ID) {
		return domain.Entry{}, domain.ErrNotFound
	}
	return e, nil
}

// editable returns an entry the actor may change: their own, or a shared one.
func (s *Entry) editable(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Entry, error) {
	e, err := s.load(ctx, actor, id)
	if err != nil {
		return domain.Entry{}, err
	}
	if e.AuthorID != actor.ID && e.Visibility != domain.VisibilityShared {
		return domain.Entry{}, fmt.Errorf("%w: not your entry", domain.ErrForbidden)
	}
	return e, nil
}

// check validates and normalises what was asked for an entry of the kind.
func (s *Entry) check(ctx context.Context, householdID uuid.UUID, kind domain.EntryKind, f domain.EntryFields) (domain.EntryFields, error) {
	maxLen := domain.MaxNoteText
	switch kind {
	case domain.KindReminder:
		maxLen = domain.MaxReminderText
	case domain.KindEvent:
		maxLen = domain.MaxEventText
	}
	f.Text = strings.TrimSpace(f.Text)
	if f.Text == "" || utf8.RuneCountInString(f.Text) > maxLen {
		return f, domain.TextError{Max: maxLen}
	}
	switch f.Visibility {
	case domain.VisibilityPrivate, domain.VisibilityShared:
		f.RevealAt = nil
	case domain.VisibilityReveal:
		if kind != domain.KindNote || f.RevealAt == nil {
			return f, domain.ErrRevealAt
		}
	default:
		return f, fmt.Errorf("%w: unknown visibility %q", domain.ErrInvalid, f.Visibility)
	}
	if f.Repeat == "" {
		f.Repeat = domain.RepeatNever
	}
	eventFields := f.StartsAt != nil || f.EndsAt != nil || f.AllDay || f.AlertMinutes != nil
	switch kind {
	case domain.KindNote:
		if f.DueAt != nil || f.Repeat != domain.RepeatNever || f.AssigneeID != nil || eventFields {
			return f, domain.ErrEntryFields
		}
	case domain.KindReminder:
		if (f.Repeat != domain.RepeatNever && f.DueAt == nil) || eventFields {
			return f, domain.ErrEntryFields
		}
	case domain.KindEvent:
		if f.AssigneeID != nil {
			return f, domain.ErrEntryFields
		}
		if f.StartsAt == nil || f.EndsAt != nil && f.EndsAt.Before(*f.StartsAt) ||
			f.AlertMinutes != nil && (*f.AlertMinutes < 0 || *f.AlertMinutes > domain.MaxAlertMinutes) {
			return f, domain.ErrEventTime
		}
		if f.AllDay {
			f.StartsAt = s.midnight(*f.StartsAt)
			if f.EndsAt != nil {
				f.EndsAt = s.midnight(*f.EndsAt)
			}
		}
		// The service decides when an event's alert goes out.
		f.DueAt = nil
	default:
		return f, fmt.Errorf("%w: unknown kind %q", domain.ErrInvalid, kind)
	}
	if f.Bill != nil {
		b := f.Bill
		if kind != domain.KindReminder || f.Visibility != domain.VisibilityShared || s.bills == nil ||
			b.Cents < 1 || b.Cents > domain.MaxExpenseCents || !b.Category.Valid() {
			return f, domain.ErrBill
		}
		if b.PaidBy != nil {
			members, err := s.households.Members(ctx, householdID)
			if err != nil {
				return f, err
			}
			if !slices.ContainsFunc(members, func(m domain.Member) bool { return m.User.ID == *b.PaidBy }) {
				return f, domain.ErrBill
			}
		}
	}
	if f.AssigneeID != nil {
		if f.Visibility != domain.VisibilityShared {
			return f, domain.ErrAssignee
		}
		members, err := s.households.Members(ctx, householdID)
		if err != nil {
			return f, err
		}
		if !slices.ContainsFunc(members, func(m domain.Member) bool { return m.User.ID == *f.AssigneeID }) {
			return f, domain.ErrAssignee
		}
	}
	return f, nil
}

// schedule sets when a new or moved entry next notifies. A time that has already passed counts as
// notified, so setting a reminder for a moment ago doesn't ring at once.
func (s *Entry) schedule(e *domain.Entry) {
	now := s.now()
	if e.Kind == domain.KindEvent {
		e.DueAt = s.nextAlert(e.EntryFields, now)
	}
	if e.DueAt != nil && !e.DueAt.After(now) {
		e.NotifiedAt = &now
	}
}

// nextAlert is when an event's alert goes out: before its start, or for a repeating event before
// its first time whose alert is still to come. Nil means no alert.
func (s *Entry) nextAlert(f domain.EntryFields, now time.Time) *time.Time {
	if f.AlertMinutes == nil || f.StartsAt == nil {
		return nil
	}
	before := time.Duration(*f.AlertMinutes) * time.Minute
	start := *f.StartsAt
	if f.Repeat != domain.RepeatNever && !start.Add(-before).After(now) {
		start = f.Repeat.Next(start, now.Add(before), s.loc)
	}
	alert := start.Add(-before)
	return &alert
}

func (s *Entry) midnight(t time.Time) *time.Time {
	y, m, d := t.In(s.loc).Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, s.loc)
	return &day
}

func (s *Entry) HasCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) (bool, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return false, err
	}
	return s.feeds.HasCalendarFeed(ctx, domain.CalendarFeed{UserID: actor.ID, HouseholdID: householdID})
}

func (s *Entry) CreateCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return "", err
	}
	token := randomID()
	if err := s.feeds.SaveCalendarFeed(ctx, hashCode(token), domain.CalendarFeed{UserID: actor.ID, HouseholdID: householdID}); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Entry) DeleteCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.feeds.DeleteCalendarFeed(ctx, domain.CalendarFeed{UserID: actor.ID, HouseholdID: householdID})
}

func (s *Entry) CalendarFeed(ctx context.Context, token string) ([]domain.Entry, *time.Location, error) {
	if token == "" {
		return nil, nil, domain.ErrNotFound
	}
	f, err := s.feeds.CalendarFeed(ctx, hashCode(token))
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.households.Role(ctx, f.HouseholdID, f.UserID); err != nil {
		return nil, nil, err
	}
	es, err := s.entries.ListEntries(ctx, f.HouseholdID, f.UserID, "", s.now().Add(-feedHistory), feedLimit)
	if err != nil {
		return nil, nil, err
	}
	out := []domain.Entry{}
	for _, e := range es {
		switch {
		case !visible(e, f.UserID):
		case e.Kind == domain.KindEvent, e.Kind == domain.KindReminder && e.DueAt != nil && e.DoneAt == nil:
			out = append(out, e)
		}
	}
	return out, s.loc, nil
}

func visible(e domain.Entry, viewer uuid.UUID) bool {
	return e.AuthorID == viewer || e.Visibility != domain.VisibilityPrivate
}

// sealEntry hides the text of someone else's "later" entry until it opens.
func sealEntry(e domain.Entry, viewer uuid.UUID, now time.Time) domain.Entry {
	if e.Visibility == domain.VisibilityReveal && e.AuthorID != viewer && e.RevealAt != nil && now.Before(*e.RevealAt) {
		e.Text, e.Sealed = "", true
	}
	return e
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
