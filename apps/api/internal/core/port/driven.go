// Package port defines the boundaries of the core: driving ports are implemented by the core
// and called by adapters (HTTP, MCP, jobs); driven ports are implemented by adapters
// (Postgres, Redis, authentik) and called by the core.
package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

type TokenVerifier interface {
	// Verify returns domain.ErrUnauthenticated for any token that is not acceptable.
	Verify(ctx context.Context, rawToken string) (domain.Identity, error)
}

// LoginProvider runs the authorization code flow for the confidential web client.
type LoginProvider interface {
	AuthCodeURL(ctx context.Context, state, nonce, verifier string) (string, error)
	// Exchange redeems the code and returns the identity and raw ID token. It returns
	// domain.ErrUnauthenticated for a code or ID token that is not acceptable.
	Exchange(ctx context.Context, code, verifier, nonce string) (domain.Identity, string, error)
	// LogoutURL is where to send the browser to sign out of the provider too; "" if it has none.
	LogoutURL(ctx context.Context, idToken string) (string, error)
}

// SessionStore keeps pending logins and sessions; it decides how long each lives.
type SessionStore interface {
	SaveLogin(ctx context.Context, state string, l domain.PendingLogin) error
	// TakeLogin returns and removes a pending login, so a state works once. It returns
	// domain.ErrNotFound for an unknown or expired state.
	TakeLogin(ctx context.Context, state string) (domain.PendingLogin, error)
	Save(ctx context.Context, id string, s domain.Session) error
	// Get returns domain.ErrNotFound for an unknown or expired session.
	Get(ctx context.Context, id string) (domain.Session, error)
	Delete(ctx context.Context, id string) error
	SaveHomeConnection(ctx context.Context, state string, p domain.PendingHomeConnection) error
	// TakeHomeConnection works once per state, like TakeLogin.
	TakeHomeConnection(ctx context.Context, state string) (domain.PendingHomeConnection, error)
}

type UserRepository interface {
	UpsertByIdentity(ctx context.Context, id domain.Identity) (domain.User, error)
}

type HouseholdRepository interface {
	// Create stores the household and makes owner its first member, atomically.
	Create(ctx context.Context, name string, owner uuid.UUID) (domain.Membership, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]domain.Membership, error)
	// Role returns domain.ErrNotFound when the user is not a member.
	Role(ctx context.Context, householdID, userID uuid.UUID) (domain.Role, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Household, error)
	Members(ctx context.Context, householdID uuid.UUID) ([]domain.Member, error)
	CreateInvite(ctx context.Context, householdID, createdBy uuid.UUID, codeHash []byte, expiresAt time.Time) error
	// AcceptInvite adds the user to the invite's household and uses the invite up, atomically.
	// An existing member gets the membership back and the invite stays unused. It returns
	// domain.ErrNotFound for an unknown, used or expired invite.
	AcceptInvite(ctx context.Context, codeHash []byte, userID uuid.UUID) (domain.Membership, error)
}

type QuestionRepository interface {
	CreateQuestion(ctx context.Context, householdID, askedBy uuid.UUID, text string) (domain.Question, error)
	// GetQuestion returns domain.ErrNotFound for an unknown id.
	GetQuestion(ctx context.Context, id uuid.UUID) (domain.Question, error)
	// ListQuestions returns the newest questions first, with their answers.
	ListQuestions(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.Question, error)
	// Answer stores or replaces the user's answer and, in the same transaction, reveals the
	// question once every member of its household (at least two) has answered. It returns
	// domain.ErrConflict if the question was already revealed.
	Answer(ctx context.Context, questionID, userID uuid.UUID, text string) (domain.Question, error)
}

// RecipeBook is the household's recipe collection and shopping list (Mealie). Every method
// returns domain.ErrUnavailable when the service can't be reached.
type RecipeBook interface {
	// Recipes returns the summaries of all recipes (up to a few hundred).
	Recipes(ctx context.Context) ([]domain.RecipeSummary, error)
	// Recipe returns domain.ErrNotFound for an unknown slug.
	Recipe(ctx context.Context, slug string) (domain.Recipe, error)
	// AddToShoppingList puts the recipe's ingredients with the given ids on the shopping list.
	// Unknown ids are ignored; domain.ErrNotFound means the recipe or the list is missing.
	AddToShoppingList(ctx context.Context, slug string, ingredientIDs []string) (domain.ShoppingListAdded, error)
}

// Chef is the LLM's cooking judgement.
type Chef interface {
	// Pick chooses up to n of the candidates that fit the mood and says why, in the language
	// given as a BCP 47 tag. It returns domain.ErrUnavailable when the LLM can't be asked.
	Pick(ctx context.Context, mood, lang string, candidates []domain.RecipeSummary, n int) ([]domain.Pick, error)
}

// SmartHome is Home Assistant, narrowed to what Homeloom may do: read scenes and lights, turn a
// scene on, switch a light. Locks, alarms, covers and other domains are deliberately not here.
// Calls act as the person who signed in, through the refresh token from that sign-in. Every
// method returns domain.ErrUnavailable when Home Assistant can't be reached, and
// domain.ErrUnauthenticated when it no longer accepts the refresh token.
type SmartHome interface {
	// AuthorizeURL is Home Assistant's sign-in page; it redirects back with a code and the state.
	AuthorizeURL(state string) string
	// Exchange redeems the code and returns the refresh token.
	Exchange(ctx context.Context, code string) (string, error)
	Revoke(ctx context.Context, refreshToken string) error
	Devices(ctx context.Context, refreshToken string) ([]domain.Scene, []domain.Light, error)
	// ActivateScene and ToggleLight take an id that Devices returned.
	ActivateScene(ctx context.Context, refreshToken, id string) error
	ToggleLight(ctx context.Context, refreshToken, id string) (domain.Light, error)
}

type HomeConnectionRepository interface {
	// SaveHomeConnection replaces the person's connection.
	SaveHomeConnection(ctx context.Context, c domain.HomeConnection) error
	// HomeConnection returns domain.ErrNotFound when the person has none.
	HomeConnection(ctx context.Context, userID uuid.UUID) (domain.HomeConnection, error)
	DeleteHomeConnection(ctx context.Context, userID uuid.UUID) error
}

type HealthCheck interface {
	Name() string
	Check(ctx context.Context) error
}
