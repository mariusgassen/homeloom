package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

const (
	// onThisDayLimit caps how many moments come back from one day in earlier years.
	onThisDayLimit = 10
	// gratitudeAge is how many days old a gratitude line is before it comes back in the app;
	// gratitudePushAge before it comes back as a notification.
	gratitudeAge     = 7
	gratitudePushAge = 14
	// gratitudeChance is how likely a past line comes back to each person on a given evening.
	gratitudeChance = 1.0 / 3
	// The local hours from which the daily notifications go out: this day in earlier years in the
	// morning, last month on the evening of the 1st, a past gratitude line later that evening.
	onThisDayHour = 9
	monthHour     = 19
	gratitudeHour = 20
)

// Moments is the household's timeline: moments, one gratitude line per person and day, and what
// comes back from them in the app and as gentle notifications.
type Moments struct {
	households port.HouseholdRepository
	moments    port.MomentRepository
	jobs       port.JobRepository
	notify     notifier
	loc        *time.Location
	now        func() time.Time
	chance     func() float64
	// photos is where photos go; nil keeps them in the moment repository.
	photos port.PhotoStore
	// videos is where videos go; without it moments have none.
	videos port.VideoStore
}

// NewMoments counts days in loc, the household's time zone.
func NewMoments(households port.HouseholdRepository, moments port.MomentRepository, jobs port.JobRepository, notify notifier, loc *time.Location) *Moments {
	return &Moments{households: households, moments: moments, jobs: jobs, notify: notify, loc: loc, now: time.Now, chance: rand.Float64}
}

// WithPhotos keeps new photos in photos instead of the moment repository. Photos from before
// stay where they are and are still served from there.
func (s *Moments) WithPhotos(photos port.PhotoStore) *Moments {
	s.photos = photos
	return s
}

// WithVideos lets moments have videos, kept in videos.
func (s *Moments) WithVideos(videos port.VideoStore) *Moments {
	s.videos = videos
	return s
}

func (s *Moments) Month(ctx context.Context, actor domain.User, householdID uuid.UUID, day time.Time) (domain.MomentMonth, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.MomentMonth{}, err
	}
	if day.IsZero() {
		day = s.today()
	}
	from := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	ms, err := s.moments.ListMoments(ctx, householdID, actor.ID, from, from.AddDate(0, 1, 0), domain.MaxMomentsPerMonth)
	if err != nil {
		return domain.MomentMonth{}, err
	}
	now := s.now()
	out := domain.MomentMonth{Month: from, Moments: make([]domain.Moment, 0, len(ms))}
	for _, m := range ms {
		if visibleMoment(m, actor.ID) {
			out.Moments = append(out.Moments, sealMoment(m, actor.ID, now))
		}
	}
	earlier, err := s.moments.LatestMomentDay(ctx, householdID, actor.ID, from)
	switch {
	case err == nil:
		out.Earlier = time.Date(earlier.Year(), earlier.Month(), 1, 0, 0, 0, 0, time.UTC)
	case !errors.Is(err, domain.ErrNotFound):
		return domain.MomentMonth{}, err
	}
	return out, nil
}

func (s *Moments) Create(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.MomentKind, f domain.MomentFields) (domain.Moment, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.Moment{}, err
	}
	if kind == domain.KindGratitude {
		f.Day = s.today()
	}
	f, err := s.check(kind, f)
	if err != nil {
		return domain.Moment{}, err
	}
	m := domain.Moment{HouseholdID: householdID, AuthorID: actor.ID, Kind: kind, MomentFields: f}
	if kind == domain.KindGratitude {
		return s.moments.SaveGratitude(ctx, m)
	}
	return s.moments.CreateMoment(ctx, m)
}

func (s *Moments) Get(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Moment, error) {
	m, err := s.load(ctx, actor, id)
	if err != nil {
		return domain.Moment{}, err
	}
	return sealMoment(m, actor.ID, s.now()), nil
}

func (s *Moments) Update(ctx context.Context, actor domain.User, id uuid.UUID, f domain.MomentFields) (domain.Moment, error) {
	m, err := s.own(ctx, actor, id)
	if err != nil {
		return domain.Moment{}, err
	}
	if m.Kind == domain.KindGratitude {
		f.Day = m.Day
	}
	if f, err = s.check(m.Kind, f); err != nil {
		return domain.Moment{}, err
	}
	m.MomentFields = f
	return s.moments.UpdateMoment(ctx, m)
}

func (s *Moments) Delete(ctx context.Context, actor domain.User, id uuid.UUID) error {
	m, err := s.own(ctx, actor, id)
	if err != nil {
		return err
	}
	// The photo goes first, so a deleted moment never leaves its photo behind.
	if s.photos != nil && m.PhotoAt != nil {
		if err := s.photos.DeletePhoto(ctx, photoKey(id)); err != nil {
			return err
		}
	}
	if s.videos != nil && m.VideoAt != nil {
		if err := s.videos.DeleteVideo(ctx, videoKey(id)); err != nil {
			return err
		}
	}
	return s.moments.DeleteMoment(ctx, id)
}

func (s *Moments) Resurfaced(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Moment, *domain.Moment, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, nil, err
	}
	today := s.today()
	ms, err := s.onThisDay(ctx, householdID, actor.ID, today)
	if err != nil {
		return nil, nil, err
	}
	line, err := s.pastGratitude(ctx, householdID, actor.ID, today.AddDate(0, 0, -gratitudeAge))
	if err != nil {
		return nil, nil, err
	}
	return ms, line, nil
}

func (s *Moments) SetPhoto(ctx context.Context, actor domain.User, id uuid.UUID, p domain.Photo) (domain.Moment, error) {
	m, err := s.own(ctx, actor, id)
	if err != nil {
		return domain.Moment{}, err
	}
	// The app sends JPEGs; checking the bytes as well keeps anything else from being served as one.
	if m.Kind != domain.KindMoment || p.ContentType != "image/jpeg" || len(p.Data) > domain.MaxPhotoBytes ||
		!bytes.HasPrefix(p.Data, []byte{0xFF, 0xD8, 0xFF}) {
		return domain.Moment{}, domain.ErrPhoto
	}
	if s.photos != nil {
		if err := s.photos.PutPhoto(ctx, photoKey(id), p); err != nil {
			return domain.Moment{}, err
		}
		if err := s.moments.MarkMomentPhoto(ctx, id); err != nil {
			return domain.Moment{}, err
		}
		return s.moments.GetMoment(ctx, id)
	}
	if err := s.moments.SetMomentPhoto(ctx, id, p); err != nil {
		return domain.Moment{}, err
	}
	return s.moments.GetMoment(ctx, id)
}

func (s *Moments) Photo(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Photo, error) {
	m, err := s.Get(ctx, actor, id)
	if err != nil {
		return domain.Photo{}, err
	}
	if m.Sealed || m.PhotoAt == nil {
		return domain.Photo{}, domain.ErrNotFound
	}
	if s.photos != nil {
		// A photo set before the store was there is still in the repository.
		p, err := s.photos.GetPhoto(ctx, photoKey(id))
		if !errors.Is(err, domain.ErrNotFound) {
			return p, err
		}
	}
	return s.moments.MomentPhoto(ctx, id)
}

func (s *Moments) DeletePhoto(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Moment, error) {
	if _, err := s.own(ctx, actor, id); err != nil {
		return domain.Moment{}, err
	}
	if s.photos != nil {
		if err := s.photos.DeletePhoto(ctx, photoKey(id)); err != nil {
			return domain.Moment{}, err
		}
	}
	if err := s.moments.DeleteMomentPhoto(ctx, id); err != nil {
		return domain.Moment{}, err
	}
	return s.moments.GetMoment(ctx, id)
}

// videoTypes are the videos a moment can have: what phones record and browsers play.
var videoTypes = map[string]bool{"video/mp4": true, "video/quicktime": true}

func (s *Moments) SetVideo(ctx context.Context, actor domain.User, id uuid.UUID, contentType string, r io.Reader) (domain.Moment, error) {
	m, err := s.own(ctx, actor, id)
	if err != nil {
		return domain.Moment{}, err
	}
	if s.videos == nil {
		return domain.Moment{}, fmt.Errorf("%w: videos need the S3 store", domain.ErrUnavailable)
	}
	if m.Kind != domain.KindMoment || !videoTypes[contentType] {
		return domain.Moment{}, domain.ErrVideo
	}
	// Both are ISO base media files, which start with a box of type "ftyp"; checking that keeps
	// anything else from being served as a video.
	br := bufio.NewReader(r)
	if head, err := br.Peek(8); err != nil || string(head[4:8]) != "ftyp" {
		return domain.Moment{}, domain.ErrVideo
	}
	if err := s.videos.PutVideo(ctx, videoKey(id), contentType, &capped{r: br, left: domain.MaxVideoBytes}); err != nil {
		if errors.Is(err, domain.ErrVideo) {
			return domain.Moment{}, domain.ErrVideo
		}
		return domain.Moment{}, err
	}
	if err := s.moments.MarkMomentVideo(ctx, id); err != nil {
		return domain.Moment{}, err
	}
	return s.moments.GetMoment(ctx, id)
}

func (s *Moments) Video(ctx context.Context, actor domain.User, id uuid.UUID) (domain.VideoStream, error) {
	m, err := s.Get(ctx, actor, id)
	if err != nil {
		return domain.VideoStream{}, err
	}
	if m.Sealed || m.VideoAt == nil || s.videos == nil {
		return domain.VideoStream{}, domain.ErrNotFound
	}
	key := videoKey(id)
	v, err := s.videos.StatVideo(ctx, key)
	if err != nil {
		return domain.VideoStream{}, err
	}
	return domain.VideoStream{Video: v, Open: func(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
		return s.videos.OpenVideo(ctx, key, offset, length)
	}}, nil
}

func (s *Moments) DeleteVideo(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Moment, error) {
	m, err := s.own(ctx, actor, id)
	if err != nil {
		return domain.Moment{}, err
	}
	if s.videos != nil && m.VideoAt != nil {
		if err := s.videos.DeleteVideo(ctx, videoKey(id)); err != nil {
			return domain.Moment{}, err
		}
	}
	if err := s.moments.ClearMomentVideo(ctx, id); err != nil {
		return domain.Moment{}, err
	}
	return s.moments.GetMoment(ctx, id)
}

// capped reads at most left bytes and fails with domain.ErrVideo on more, so a video that is too
// big is stopped while it is sent, not after.
type capped struct {
	r    io.Reader
	left int64
}

func (c *capped) Read(p []byte) (int, error) {
	if c.left <= 0 {
		// One byte more tells a video that is exactly the limit from a bigger one.
		var b [1]byte
		n, err := c.r.Read(b[:])
		if n > 0 {
			return 0, domain.ErrVideo
		}
		return 0, err
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}

// NotifyDaily sends the day's notifications once they are due: this day in earlier years, last
// month's look back on the 1st, and now and then a past gratitude line. It is meant to run every
// minute or so; each goes out once per day, also with several API instances.
func (s *Moments) NotifyDaily(ctx context.Context) error {
	now := s.now().In(s.loc)
	today := s.today()
	var errs []error
	if now.Hour() >= onThisDayHour {
		errs = append(errs, s.daily(ctx, "on-this-day", today, s.notifyOnThisDay))
	}
	if now.Day() == 1 && now.Hour() >= monthHour {
		errs = append(errs, s.daily(ctx, "last-month", today, s.notifyLastMonth))
	}
	if now.Hour() >= gratitudeHour {
		errs = append(errs, s.daily(ctx, "gratitude", today, s.notifyGratitude))
	}
	return errors.Join(errs...)
}

// daily runs notify for every member of every household, if this instance is the first to claim
// the job today.
func (s *Moments) daily(ctx context.Context, job string, today time.Time, notify func(context.Context, uuid.UUID, domain.User, []domain.Member, time.Time) error) error {
	claimed, err := s.jobs.ClaimJobRun(ctx, job, today)
	if err != nil || !claimed {
		return err
	}
	ids, err := s.households.HouseholdIDs(ctx)
	if err != nil {
		return err
	}
	for _, hh := range ids {
		members, err := s.households.Members(ctx, hh)
		if err != nil {
			slog.WarnContext(ctx, "moments: list members", "err", err, "job", job)
			continue
		}
		for _, m := range members {
			if err := notify(ctx, hh, m.User, members, today); err != nil {
				slog.WarnContext(ctx, "moments: notify", "err", err, "job", job)
			}
		}
	}
	return nil
}

func (s *Moments) notifyOnThisDay(ctx context.Context, householdID uuid.UUID, u domain.User, _ []domain.Member, today time.Time) error {
	ms, err := s.onThisDay(ctx, householdID, u.ID, today)
	if err != nil || len(ms) == 0 {
		return err
	}
	m := ms[0]
	s.notify.Notify(ctx, []uuid.UUID{u.ID}, domain.Notification{
		Key: "push.on_this_day", Args: []any{"year", m.Day.Year(), "text", m.Text}, URL: "/us", Tag: "on-this-day",
	})
	return nil
}

func (s *Moments) notifyLastMonth(ctx context.Context, householdID uuid.UUID, u domain.User, _ []domain.Member, today time.Time) error {
	to := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, -1, 0)
	ms, err := s.moments.ListMoments(ctx, householdID, u.ID, from, to, 1)
	if err != nil || len(ms) == 0 {
		return err
	}
	month := from.Format("2006-01")
	s.notify.Notify(ctx, []uuid.UUID{u.ID}, domain.Notification{Key: "push.last_month", URL: "/us?month=" + month, Tag: "month-" + month})
	return nil
}

func (s *Moments) notifyGratitude(ctx context.Context, householdID uuid.UUID, u domain.User, members []domain.Member, today time.Time) error {
	if s.chance() >= gratitudeChance {
		return nil
	}
	line, err := s.pastGratitude(ctx, householdID, u.ID, today.AddDate(0, 0, -gratitudePushAge))
	if err != nil || line == nil {
		return err
	}
	n := domain.Notification{Key: "push.gratitude_own", Args: []any{"text", line.Text}, URL: "/us", Tag: "gratitude"}
	if line.AuthorID != u.ID {
		n.Key = "push.gratitude"
		for _, m := range members {
			if m.User.ID == line.AuthorID {
				n.Args = append(n.Args, "name", m.User.Name)
			}
		}
	}
	s.notify.Notify(ctx, []uuid.UUID{u.ID}, n)
	return nil
}

// onThisDay returns what the viewer can read from today's date in earlier years.
func (s *Moments) onThisDay(ctx context.Context, householdID, viewer uuid.UUID, today time.Time) ([]domain.Moment, error) {
	ms, err := s.moments.MomentsOnDay(ctx, householdID, viewer, today, onThisDayLimit)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := []domain.Moment{}
	for _, m := range ms {
		if m = sealMoment(m, viewer, now); visibleMoment(m, viewer) && !m.Sealed {
			out = append(out, m)
		}
	}
	return out, nil
}

// pastGratitude picks a gratitude line the viewer can see from before the day, or nil.
func (s *Moments) pastGratitude(ctx context.Context, householdID, viewer uuid.UUID, before time.Time) (*domain.Moment, error) {
	m, err := s.moments.RandomGratitude(ctx, householdID, viewer, before)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	case !visibleMoment(m, viewer):
		return nil, nil
	}
	return &m, nil
}

// load returns a moment the actor may see, unsealed; anything else is domain.ErrNotFound.
func (s *Moments) load(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Moment, error) {
	m, err := s.moments.GetMoment(ctx, id)
	if err != nil {
		return domain.Moment{}, err
	}
	if _, err := s.households.Role(ctx, m.HouseholdID, actor.ID); err != nil {
		return domain.Moment{}, err
	}
	if !visibleMoment(m, actor.ID) {
		return domain.Moment{}, domain.ErrNotFound
	}
	return m, nil
}

// own returns a moment the actor wrote. Moments and lines are personal, so only their author
// changes them, also when they are shared.
func (s *Moments) own(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Moment, error) {
	m, err := s.load(ctx, actor, id)
	if err != nil {
		return domain.Moment{}, err
	}
	if m.AuthorID != actor.ID {
		return domain.Moment{}, fmt.Errorf("%w: not your moment", domain.ErrForbidden)
	}
	return m, nil
}

// check validates and normalises what was asked for a moment of the kind.
func (s *Moments) check(kind domain.MomentKind, f domain.MomentFields) (domain.MomentFields, error) {
	maxLen := domain.MaxMomentText
	switch kind {
	case domain.KindMoment:
	case domain.KindGratitude:
		maxLen = domain.MaxGratitudeText
	default:
		return f, fmt.Errorf("%w: unknown kind %q", domain.ErrInvalid, kind)
	}
	f.Text = strings.TrimSpace(f.Text)
	if f.Text == "" || utf8.RuneCountInString(f.Text) > maxLen {
		return f, domain.TextError{Max: maxLen}
	}
	switch f.Visibility {
	case domain.VisibilityPrivate, domain.VisibilityShared:
		f.RevealAt = nil
	case domain.VisibilityReveal:
		if kind != domain.KindMoment || f.RevealAt == nil {
			return f, domain.ErrRevealAt
		}
	default:
		return f, fmt.Errorf("%w: unknown visibility %q", domain.ErrInvalid, f.Visibility)
	}
	today := s.today()
	if f.Day.IsZero() {
		f.Day = today
	}
	f.Day = time.Date(f.Day.Year(), f.Day.Month(), f.Day.Day(), 0, 0, 0, 0, time.UTC)
	if f.Day.After(today) {
		return f, domain.ErrMomentDay
	}
	return f, nil
}

// today is the household's date, as midnight UTC like Moment.Day.
func (s *Moments) today() time.Time {
	y, m, d := s.now().In(s.loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// photoKey is where a moment's photo is kept in the PhotoStore.
func photoKey(id uuid.UUID) string { return "moments/" + id.String() + ".jpg" }

// videoKey is where a moment's video is kept in the VideoStore.
func videoKey(id uuid.UUID) string { return "moments/" + id.String() + ".video" }

func visibleMoment(m domain.Moment, viewer uuid.UUID) bool {
	return m.AuthorID == viewer || m.Visibility != domain.VisibilityPrivate
}

// sealMoment hides the text, photo and video of someone else's "later" moment until it opens.
func sealMoment(m domain.Moment, viewer uuid.UUID, now time.Time) domain.Moment {
	if m.Visibility == domain.VisibilityReveal && m.AuthorID != viewer && m.RevealAt != nil && now.Before(*m.RevealAt) {
		m.Text, m.PhotoAt, m.VideoAt, m.Sealed = "", nil, nil, true
	}
	return m
}
