package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// syncHistory is how far back a calendar app sees events that are over.
const syncHistory = 365 * 24 * time.Hour

func (s *Entry) SyncEntries(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList) ([]domain.Entry, error) {
	if err := checkList(l); err != nil {
		return nil, err
	}
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	es, err := s.entries.ListEntries(ctx, householdID, actor.ID, l.Kind, s.now().Add(-syncHistory), feedLimit)
	if err != nil {
		return nil, err
	}
	out := []domain.Entry{}
	for _, e := range es {
		if inList(e, actor.ID, l) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *Entry) SyncEntry(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList, name string) (domain.Entry, error) {
	if err := checkList(l); err != nil {
		return domain.Entry{}, err
	}
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Entry{}, err
	}
	e, err := s.byName(ctx, householdID, name)
	if err != nil {
		return domain.Entry{}, err
	}
	if !inList(e, actor.ID, l) {
		return domain.Entry{}, domain.ErrNotFound
	}
	return e, nil
}

func (s *Entry) SyncPut(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList, name string, in domain.SyncEntry) (domain.Entry, bool, error) {
	if err := checkList(l); err != nil {
		return domain.Entry{}, false, err
	}
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Entry{}, false, err
	}
	// The list decides who sees it; calendar apps know nothing of assignees or "later".
	in.Visibility, in.RevealAt, in.AssigneeID = l.Visibility, nil, nil
	e, err := s.byName(ctx, householdID, name)
	if errors.Is(err, domain.ErrNotFound) {
		e, err = s.syncCreate(ctx, actor, householdID, l, name, in)
		return e, err == nil, err
	}
	if err != nil {
		return domain.Entry{}, false, err
	}
	if !visible(e, actor.ID) || e.Kind != l.Kind {
		// The name belongs to something else.
		return domain.Entry{}, false, fmt.Errorf("%w: name in use", domain.ErrConflict)
	}
	if l.Visibility == domain.VisibilityShared {
		in.AssigneeID = e.AssigneeID
	}
	if e, err = s.Update(ctx, actor, e.ID, in.EntryFields, nil); err != nil {
		return domain.Entry{}, false, err
	}
	if l.Kind == domain.KindReminder && in.Done != (e.DoneAt != nil) {
		e, err = s.SetDone(ctx, actor, e.ID, in.Done)
	}
	return e, false, err
}

func (s *Entry) syncCreate(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList, name string, in domain.SyncEntry) (domain.Entry, error) {
	if err := checkName(name); err != nil {
		return domain.Entry{}, err
	}
	f, err := s.check(ctx, householdID, l.Kind, in.EntryFields)
	if err != nil {
		return domain.Entry{}, err
	}
	e := domain.Entry{HouseholdID: householdID, AuthorID: actor.ID, Kind: l.Kind, EntryFields: f, ICalUID: in.UID, ICalName: name}
	s.schedule(&e)
	if e, err = s.entries.CreateEntry(ctx, e); err != nil || !in.Done || l.Kind != domain.KindReminder {
		return e, err
	}
	return s.SetDone(ctx, actor, e.ID, true)
}

func (s *Entry) SyncDelete(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList, name string) error {
	if err := checkList(l); err != nil {
		return err
	}
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	e, err := s.byName(ctx, householdID, name)
	if err != nil {
		return err
	}
	switch {
	case !visible(e, actor.ID):
		return domain.ErrNotFound
	case !inList(e, actor.ID, l):
		// Calendar apps move an entry to another list by putting it there and then deleting it
		// here; by then it is no longer here.
		return nil
	}
	return s.Delete(ctx, actor, e.ID)
}

// SyncMove moves the named entry to another list of the same kind and gives it a new name. Only
// its author may move it between shared and private.
func (s *Entry) SyncMove(ctx context.Context, actor domain.User, householdID uuid.UUID, from domain.SyncList, name string, to domain.SyncList, newName string) (domain.Entry, error) {
	if err := checkList(from); err != nil {
		return domain.Entry{}, err
	}
	if err := checkList(to); err != nil {
		return domain.Entry{}, err
	}
	if from.Kind != to.Kind {
		return domain.Entry{}, fmt.Errorf("%w: an entry stays the kind it is", domain.ErrInvalid)
	}
	if err := checkName(newName); err != nil {
		return domain.Entry{}, err
	}
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Entry{}, err
	}
	e, err := s.byName(ctx, householdID, name)
	if err != nil {
		return domain.Entry{}, err
	}
	if !inList(e, actor.ID, from) {
		return domain.Entry{}, domain.ErrNotFound
	}
	switch other, err := s.byName(ctx, householdID, newName); {
	case err == nil && other.ID != e.ID:
		return domain.Entry{}, fmt.Errorf("%w: name in use", domain.ErrConflict)
	case err != nil && !errors.Is(err, domain.ErrNotFound):
		return domain.Entry{}, err
	}
	if to.Visibility != e.Visibility {
		f := e.EntryFields
		f.Visibility, f.AssigneeID = to.Visibility, nil
		if e, err = s.Update(ctx, actor, e.ID, f, nil); err != nil {
			return domain.Entry{}, err
		}
	} else if e, err = s.editable(ctx, actor, e.ID); err != nil {
		return domain.Entry{}, err
	}
	if err := s.entries.RenameEntry(ctx, e.ID, newName); err != nil {
		return domain.Entry{}, err
	}
	e.ICalName = newName
	return e, nil
}

func checkName(name string) error {
	if name == "" || strings.Contains(name, "/") || utf8.RuneCountInString(name) > domain.MaxICalName {
		return fmt.Errorf("%w: name must be 1-%d characters without a slash", domain.ErrInvalid, domain.MaxICalName)
	}
	return nil
}

// byName finds a household's entry by the name a calendar app knows it by.
func (s *Entry) byName(ctx context.Context, householdID uuid.UUID, name string) (domain.Entry, error) {
	if id, err := uuid.Parse(strings.TrimSuffix(name, ".ics")); err == nil && strings.HasSuffix(name, ".ics") {
		e, err := s.entries.GetEntry(ctx, id)
		if err == nil && e.HouseholdID == householdID && e.ICalName == "" {
			return e, nil
		}
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return domain.Entry{}, err
		}
	}
	return s.entries.EntryByICalName(ctx, householdID, name)
}

func checkList(l domain.SyncList) error {
	if (l.Kind != domain.KindEvent && l.Kind != domain.KindReminder && l.Kind != domain.KindNote) ||
		(l.Visibility != domain.VisibilityShared && l.Visibility != domain.VisibilityPrivate) {
		return fmt.Errorf("%w: no such list", domain.ErrNotFound)
	}
	return nil
}

func inList(e domain.Entry, viewer uuid.UUID, l domain.SyncList) bool {
	return e.Kind == l.Kind && e.Visibility == l.Visibility && visible(e, viewer)
}
