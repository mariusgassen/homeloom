package service

import (
	"context"
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
	// giftNudgeHour is the local hour from which the birthday nudges go out.
	giftNudgeHour = 9
)

// giftNudgeDays are how many days before a birthday the others are nudged.
var giftNudgeDays = []int{7, 1}

// Gifts keeps each member's private gift ideas for the others. Privacy is enforced here and in
// the repository: an idea is only ever read, changed or counted for its author, so neither the
// API, the assistant nor a notification can show it to anyone else.
type Gifts struct {
	households port.HouseholdRepository
	gifts      port.GiftRepository
	jobs       port.JobRepository
	notify     notifier
	loc        *time.Location
	now        func() time.Time
}

// NewGifts counts days in loc, the household's time zone.
func NewGifts(households port.HouseholdRepository, gifts port.GiftRepository, jobs port.JobRepository, notify notifier, loc *time.Location) *Gifts {
	return &Gifts{households: households, gifts: gifts, jobs: jobs, notify: notify, loc: loc, now: time.Now}
}

func (s *Gifts) today() time.Time {
	n := s.now().In(s.loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Gifts) members(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Member, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	return s.households.Members(ctx, householdID)
}

func (s *Gifts) Overview(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Gifts, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.Gifts{}, err
	}
	birthdays, err := s.gifts.Birthdays(ctx, householdID)
	if err != nil {
		return domain.Gifts{}, err
	}
	ideas, err := s.gifts.GiftIdeas(ctx, householdID, actor.ID, domain.MaxGiftIdeas)
	if err != nil {
		return domain.Gifts{}, err
	}
	occasions, err := s.gifts.Occasions(ctx, householdID)
	if err != nil {
		return domain.Gifts{}, err
	}
	out := domain.Gifts{Ideas: ideas}
	if b, ok := birthdays[actor.ID]; ok {
		out.MyBirthday = &b
	}
	today := s.today()
	for _, o := range occasions {
		next, days := o.Date.Next(today)
		out.Occasions = append(out.Occasions, domain.GiftOccasion{Occasion: o, Next: next, Days: days})
	}
	slices.SortStableFunc(out.Occasions, func(a, b domain.GiftOccasion) int { return a.Days - b.Days })
	for _, m := range members {
		if m.User.ID == actor.ID {
			continue
		}
		p := domain.GiftPerson{User: m.User}
		if b, ok := birthdays[m.User.ID]; ok {
			p.Birthday = &b
			p.Next, p.Days = b.Next(today)
		}
		for _, i := range ideas {
			if i.ForID == m.User.ID && i.GivenAt == nil {
				p.Ideas++
			}
		}
		out.People = append(out.People, p)
	}
	// Soonest birthday first; people without one last, by name.
	slices.SortStableFunc(out.People, func(a, b domain.GiftPerson) int {
		switch {
		case a.Birthday != nil && b.Birthday != nil:
			return a.Days - b.Days
		case a.Birthday != nil:
			return -1
		case b.Birthday != nil:
			return 1
		}
		return strings.Compare(a.User.Name, b.User.Name)
	})
	return out, nil
}

func (s *Gifts) SetBirthday(ctx context.Context, actor domain.User, b *domain.Birthday) error {
	if b != nil && !b.Valid() {
		return domain.ErrBirthday
	}
	return s.gifts.SetBirthday(ctx, actor.ID, b)
}

func (s *Gifts) Add(ctx context.Context, actor domain.User, householdID, forID uuid.UUID, occasionID *uuid.UUID, text string) (domain.GiftIdea, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.GiftIdea{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > domain.MaxGiftText {
		return domain.GiftIdea{}, domain.TextError{Max: domain.MaxGiftText}
	}
	if forID == actor.ID || !slices.ContainsFunc(members, func(m domain.Member) bool { return m.User.ID == forID }) {
		return domain.GiftIdea{}, domain.ErrGiftFor
	}
	if occasionID != nil {
		occasions, err := s.gifts.Occasions(ctx, householdID)
		if err != nil {
			return domain.GiftIdea{}, err
		}
		if !slices.ContainsFunc(occasions, func(o domain.Occasion) bool { return o.ID == *occasionID }) {
			return domain.GiftIdea{}, domain.ErrOccasion
		}
	}
	have, err := s.gifts.GiftIdeas(ctx, householdID, actor.ID, domain.MaxGiftIdeas)
	if err != nil {
		return domain.GiftIdea{}, err
	}
	if len(have) >= domain.MaxGiftIdeas {
		return domain.GiftIdea{}, fmt.Errorf("%w: at most %d ideas", domain.ErrInvalid, domain.MaxGiftIdeas)
	}
	return s.gifts.AddGiftIdea(ctx, domain.GiftIdea{HouseholdID: householdID, AuthorID: actor.ID, ForID: forID, OccasionID: occasionID, Text: text})
}

func (s *Gifts) AddOccasion(ctx context.Context, actor domain.User, householdID uuid.UUID, name string, date domain.Birthday) (domain.Occasion, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Occasion{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > domain.MaxOccasionName {
		return domain.Occasion{}, domain.TextError{Max: domain.MaxOccasionName}
	}
	if !date.Valid() {
		return domain.Occasion{}, domain.ErrBirthday
	}
	have, err := s.gifts.Occasions(ctx, householdID)
	if err != nil {
		return domain.Occasion{}, err
	}
	if len(have) >= domain.MaxOccasions {
		return domain.Occasion{}, fmt.Errorf("%w: at most %d dates", domain.ErrInvalid, domain.MaxOccasions)
	}
	return s.gifts.AddOccasion(ctx, domain.Occasion{HouseholdID: householdID, Name: name, Date: date})
}

func (s *Gifts) DeleteOccasion(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.gifts.DeleteOccasion(ctx, householdID, id)
}

func (s *Gifts) SetGiven(ctx context.Context, actor domain.User, householdID, id uuid.UUID, given bool) (domain.GiftIdea, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.GiftIdea{}, err
	}
	return s.gifts.SetGiftIdeaGiven(ctx, householdID, actor.ID, id, given)
}

func (s *Gifts) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.gifts.DeleteGiftIdea(ctx, householdID, actor.ID, id)
}

// NotifyDaily tells every member, a week and a day before another member's birthday or an occasion, how many
// ideas they have for them. It goes only to the author, with only their own count. It is meant to
// run every minute or so; it goes out once per day, also with several API instances.
func (s *Gifts) NotifyDaily(ctx context.Context) error {
	if s.now().In(s.loc).Hour() < giftNudgeHour {
		return nil
	}
	today := s.today()
	claimed, err := s.jobs.ClaimJobRun(ctx, "gift-birthdays", today)
	if err != nil || !claimed {
		return err
	}
	ids, err := s.households.HouseholdIDs(ctx)
	if err != nil {
		return err
	}
	for _, hh := range ids {
		if err := s.nudge(ctx, hh, today); err != nil {
			slog.WarnContext(ctx, "gifts: nudge", "err", err)
		}
	}
	return nil
}

func (s *Gifts) nudge(ctx context.Context, householdID uuid.UUID, today time.Time) error {
	members, err := s.households.Members(ctx, householdID)
	if err != nil {
		return err
	}
	birthdays, err := s.gifts.Birthdays(ctx, householdID)
	if err != nil {
		return err
	}
	occasions, err := s.gifts.Occasions(ctx, householdID)
	if err != nil {
		return err
	}
	for _, o := range occasions {
		_, days := o.Date.Next(today)
		if !slices.Contains(giftNudgeDays, days) {
			continue
		}
		for _, m := range members {
			ideas, err := s.gifts.GiftIdeas(ctx, householdID, m.User.ID, domain.MaxGiftIdeas)
			if err != nil {
				return err
			}
			open := 0
			for _, i := range ideas {
				if i.OccasionID != nil && *i.OccasionID == o.ID && i.GivenAt == nil {
					open++
				}
			}
			key := "push.gift_occasion"
			if open == 0 {
				key = "push.gift_occasion_none"
			}
			s.notify.Notify(ctx, []uuid.UUID{m.User.ID}, domain.Notification{
				Key: key, Args: []any{"name", o.Name, "count", open}, URL: "/gifts", Tag: "gift-occasion-" + o.ID.String(),
			})
		}
	}
	for _, celebrant := range members {
		b, ok := birthdays[celebrant.User.ID]
		if !ok {
			continue
		}
		_, days := b.Next(today)
		if !slices.Contains(giftNudgeDays, days) {
			continue
		}
		for _, m := range members {
			if m.User.ID == celebrant.User.ID {
				continue
			}
			ideas, err := s.gifts.GiftIdeas(ctx, householdID, m.User.ID, domain.MaxGiftIdeas)
			if err != nil {
				return err
			}
			open := 0
			for _, i := range ideas {
				if i.ForID == celebrant.User.ID && i.GivenAt == nil {
					open++
				}
			}
			key := "push.gift_birthday"
			if open == 0 {
				key = "push.gift_birthday_none"
			}
			s.notify.Notify(ctx, []uuid.UUID{m.User.ID}, domain.Notification{
				Key: key, Args: []any{"name", celebrant.User.Name, "count", open}, URL: "/gifts", Tag: "gift-" + celebrant.User.ID.String(),
			})
		}
	}
	return nil
}
