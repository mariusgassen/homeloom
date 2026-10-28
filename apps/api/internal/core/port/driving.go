package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

type IdentityService interface {
	// Authenticate turns an access token into a local user, creating it on first sight.
	Authenticate(ctx context.Context, rawToken string) (domain.User, error)
	// AuthenticateSession does the same for a session id from the login flow.
	AuthenticateSession(ctx context.Context, sessionID string) (domain.User, error)
}

type LoginService interface {
	// Begin starts a sign-in: state is to be bound to the browser, url is where to send it.
	Begin(ctx context.Context) (state, url string, err error)
	// Complete finishes it. browserState is the state the browser was given by Begin; the
	// result is the new session's id.
	Complete(ctx context.Context, state, browserState, code string) (string, error)
	// Logout ends the session, if any, and returns the provider's sign-out URL or "".
	Logout(ctx context.Context, sessionID string) (string, error)
}

type HouseholdService interface {
	List(ctx context.Context, actor domain.User) ([]domain.Membership, error)
	Create(ctx context.Context, actor domain.User, name string) (domain.Membership, error)
	Get(ctx context.Context, actor domain.User, id uuid.UUID) (domain.HouseholdDetail, error)
	Invite(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Invite, error)
	Join(ctx context.Context, actor domain.User, code string) (domain.Membership, error)
}

type QuestionService interface {
	List(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Question, error)
	Ask(ctx context.Context, actor domain.User, householdID uuid.UUID, text string) (domain.Question, error)
	Answer(ctx context.Context, actor domain.User, questionID uuid.UUID, text string) (domain.Question, error)
}

type EntryService interface {
	List(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.EntryKind) ([]domain.Entry, error)
	Create(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.EntryKind, f domain.EntryFields) (domain.Entry, error)
	Get(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Entry, error)
	Update(ctx context.Context, actor domain.User, id uuid.UUID, f domain.EntryFields) (domain.Entry, error)
	Delete(ctx context.Context, actor domain.User, id uuid.UUID) error
	// SetDone ticks a reminder off (done true) or opens it again.
	SetDone(ctx context.Context, actor domain.User, id uuid.UUID, done bool) (domain.Entry, error)
	// HasCalendarFeed says whether the actor has a calendar subscription link for the household.
	HasCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) (bool, error)
	// CreateCalendarFeed returns a new subscription link token; an earlier link stops working.
	CreateCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error)
	DeleteCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) error
	// CalendarFeed returns the events and dated open reminders the token's owner can see, and the
	// household's time zone. It returns domain.ErrNotFound for an unknown token or a member who left.
	CalendarFeed(ctx context.Context, token string) ([]domain.Entry, *time.Location, error)
}

type CookService interface {
	// Suggest picks recipes for a mood; lang is the language the reasons are written in.
	Suggest(ctx context.Context, actor domain.User, householdID uuid.UUID, mood, lang string) (domain.Suggestions, error)
	Recipe(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string) (domain.Recipe, error)
	AddToShoppingList(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string, ingredientIDs []string) (domain.ShoppingListAdded, error)
}

type HomeService interface {
	// Connect starts the actor's sign-in at Home Assistant and returns where to send the browser.
	Connect(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error)
	// CompleteConnect finishes it for the person who started it; it returns
	// domain.ErrUnauthenticated for an unknown state or someone else's sign-in.
	CompleteConnect(ctx context.Context, actor domain.User, state, code string) error
	Disconnect(ctx context.Context, actor domain.User, householdID uuid.UUID) error
	Get(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Home, error)
	ActivateScene(ctx context.Context, actor domain.User, householdID uuid.UUID, sceneID string) error
	ToggleLight(ctx context.Context, actor domain.User, householdID uuid.UUID, lightID string) (domain.Light, error)
}

type PushService interface {
	// PublicKey returns domain.ErrUnavailable when push isn't set up.
	PublicKey(ctx context.Context) (string, error)
	// Subscribe stores the actor's device; lang is the language its notifications are written in.
	Subscribe(ctx context.Context, actor domain.User, sub domain.PushSubscription) error
	Unsubscribe(ctx context.Context, actor domain.User, endpoint string) error
}

type CallService interface {
	// Join lets the actor into their call with another member of the household, ringing that
	// member when they aren't in it yet. It returns domain.ErrNotFound when with isn't a member,
	// domain.ErrInvalid for the actor themselves, and domain.ErrUnavailable when calls aren't set
	// up or LiveKit is down.
	Join(ctx context.Context, actor domain.User, householdID, with uuid.UUID) (domain.Call, error)
}
