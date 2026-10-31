// Package port defines the boundaries of the core: driving ports are implemented by the core
// and called by adapters (HTTP, MCP, jobs); driven ports are implemented by adapters
// (Postgres, Redis, authentik) and called by the core.
package port

import (
	"context"
	"io"
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
	SaveWatchConnection(ctx context.Context, state string, p domain.PendingWatchConnection) error
	// TakeWatchConnection works once per state, like TakeLogin.
	TakeWatchConnection(ctx context.Context, state string) (domain.PendingWatchConnection, error)
	// SaveGuestPass keeps a guest link for domain.GuestPassTTL.
	SaveGuestPass(ctx context.Context, token string, p domain.GuestPass) error
	// GuestPass returns domain.ErrNotFound for an unknown or expired link; it works until it expires.
	GuestPass(ctx context.Context, token string) (domain.GuestPass, error)
	// ClaimGuestPass binds a guest link to the first device that uses it, atomically. It returns
	// domain.ErrConflict when another device already has.
	ClaimGuestPass(ctx context.Context, token, device string) error
	// CallKey keeps key as the encryption key of room unless the room has one already, and
	// returns the room's key. Each call keeps it for another domain.CallKeyTTL.
	CallKey(ctx context.Context, room, key string) (string, error)
	// SaveAgentGrant keeps the grant until it expires; AgentGrant returns domain.ErrNotFound for
	// an unknown or expired token.
	SaveAgentGrant(ctx context.Context, token string, g domain.AgentGrant) error
	AgentGrant(ctx context.Context, token string) (domain.AgentGrant, error)
	DeleteAgentGrant(ctx context.Context, token string) error
}

type UserRepository interface {
	UpsertByIdentity(ctx context.Context, id domain.Identity) (domain.User, error)
	// SetAvatar and DeleteAvatar return the user as it is afterwards; Avatar returns
	// domain.ErrNotFound when the user has set none.
	SetAvatar(ctx context.Context, userID uuid.UUID, p domain.Photo) (domain.User, error)
	Avatar(ctx context.Context, userID uuid.UUID) (domain.Photo, error)
	DeleteAvatar(ctx context.Context, userID uuid.UUID) (domain.User, error)
	// SharesHousehold reports whether a and b are members of one household together.
	SharesHousehold(ctx context.Context, a, b uuid.UUID) (bool, error)
}

type HouseholdRepository interface {
	// Create stores the household and makes owner its first member, atomically.
	Create(ctx context.Context, name string, owner uuid.UUID) (domain.Membership, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]domain.Membership, error)
	// Role returns domain.ErrNotFound when the user is not a member.
	Role(ctx context.Context, householdID, userID uuid.UUID) (domain.Role, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Household, error)
	Members(ctx context.Context, householdID uuid.UUID) ([]domain.Member, error)
	// HouseholdIDs returns every household, for jobs that visit each one.
	HouseholdIDs(ctx context.Context) ([]uuid.UUID, error)
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
	// RewordQuestion changes the text. It returns domain.ErrConflict once anyone but the asker has
	// answered, and domain.ErrNotFound for an unknown id.
	RewordQuestion(ctx context.Context, questionID uuid.UUID, text string) (domain.Question, error)
	// DeleteQuestion deletes the question with its answers, or returns domain.ErrNotFound.
	DeleteQuestion(ctx context.Context, questionID uuid.UUID) error
}

type EntryRepository interface {
	CreateEntry(ctx context.Context, e domain.Entry) (domain.Entry, error)
	// GetEntry returns domain.ErrNotFound for an unknown id.
	GetEntry(ctx context.Context, id uuid.UUID) (domain.Entry, error)
	// EntryByICalName returns the household's entry a calendar app made under that resource
	// name, or domain.ErrNotFound.
	EntryByICalName(ctx context.Context, householdID uuid.UUID, name string) (domain.Entry, error)
	// RenameEntry sets the name an app knows the entry by; domain.ErrNotFound for an unknown id.
	RenameEntry(ctx context.Context, id uuid.UUID, name string) error
	// ListEntries returns the household's entries of the kind ("" for all) that are the viewer's
	// own or not private, grouped by kind: events by start, those ending before since left out
	// unless they repeat; notes newest first; open reminders by due time, then those done in the
	// last domain.DoneReminderDays days.
	ListEntries(ctx context.Context, householdID, viewer uuid.UUID, kind domain.EntryKind, since time.Time, limit int) ([]domain.Entry, error)
	// UpdateEntry stores the entry's fields, done state and notification time.
	UpdateEntry(ctx context.Context, e domain.Entry) (domain.Entry, error)
	DeleteEntry(ctx context.Context, id uuid.UUID) error
	// ReminderSuggestions returns the titles of the household's reminders the viewer may see
	// (their own and everyone's non-private ones), case-insensitively grouped, most used first,
	// then most recent; a title with an open reminder is left out.
	ReminderSuggestions(ctx context.Context, householdID, viewer uuid.UUID, limit int) ([]domain.TitleSuggestion, error)
	// ClaimDue sets NotifiedAt to now on up to limit open entries whose DueAt (a reminder's due
	// time, an event's alert) has passed since their last notification, and returns them. Each due
	// time is claimed once, also by several API instances at the same time.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.Entry, error)
	// AddEntryPhoto stores a photo in the entry and returns its id.
	AddEntryPhoto(ctx context.Context, entryID uuid.UUID, p domain.Photo) (uuid.UUID, error)
	// EntryPhoto returns domain.ErrNotFound unless the photo is in the entry.
	EntryPhoto(ctx context.Context, entryID, photoID uuid.UUID) (domain.Photo, error)
	// EntryPhotoIDs returns the ids of the entry's photos, oldest first.
	EntryPhotoIDs(ctx context.Context, entryID uuid.UUID) ([]uuid.UUID, error)
	DeleteEntryPhotos(ctx context.Context, entryID uuid.UUID, ids []uuid.UUID) error
}

type CalendarFeedRepository interface {
	// SaveCalendarFeed replaces the member's feed for the household.
	SaveCalendarFeed(ctx context.Context, tokenHash []byte, f domain.CalendarFeed) error
	// CalendarFeed returns domain.ErrNotFound for an unknown token.
	CalendarFeed(ctx context.Context, tokenHash []byte) (domain.CalendarFeed, error)
	HasCalendarFeed(ctx context.Context, f domain.CalendarFeed) (bool, error)
	DeleteCalendarFeed(ctx context.Context, f domain.CalendarFeed) error
}

// MomentRepository keeps the household's timeline. Lists hold only what the viewer may see: their
// own moments and everyone's that are not private.
type MomentRepository interface {
	CreateMoment(ctx context.Context, m domain.Moment) (domain.Moment, error)
	// SaveGratitude stores the author's line for m.Day, replacing one they wrote that day.
	SaveGratitude(ctx context.Context, m domain.Moment) (domain.Moment, error)
	// GetMoment returns domain.ErrNotFound for an unknown id.
	GetMoment(ctx context.Context, id uuid.UUID) (domain.Moment, error)
	// UpdateMoment stores the moment's fields.
	UpdateMoment(ctx context.Context, m domain.Moment) (domain.Moment, error)
	// DeleteMoment deletes it and its photo.
	DeleteMoment(ctx context.Context, id uuid.UUID) error
	// ListMoments returns the moments with from <= Day < to, newest day first, then newest first.
	ListMoments(ctx context.Context, householdID, viewer uuid.UUID, from, to time.Time, limit int) ([]domain.Moment, error)
	// LatestMomentDay returns the newest day before the given one with a moment, or domain.ErrNotFound.
	LatestMomentDay(ctx context.Context, householdID, viewer uuid.UUID, before time.Time) (time.Time, error)
	// MomentsOnDay returns the moments on today's month and day in earlier years, newest first.
	MomentsOnDay(ctx context.Context, householdID, viewer uuid.UUID, today time.Time, limit int) ([]domain.Moment, error)
	// RandomGratitude returns one gratitude line from before the day, at random, or domain.ErrNotFound.
	RandomGratitude(ctx context.Context, householdID, viewer uuid.UUID, before time.Time) (domain.Moment, error)
	// SetMomentPhoto replaces the moment's photo and sets its PhotoAt.
	SetMomentPhoto(ctx context.Context, id uuid.UUID, p domain.Photo) error
	// MarkMomentPhoto sets PhotoAt for a photo kept in a PhotoStore and drops any copy held here.
	MarkMomentPhoto(ctx context.Context, id uuid.UUID) error
	// MomentPhoto returns domain.ErrNotFound when the moment has none.
	MomentPhoto(ctx context.Context, id uuid.UUID) (domain.Photo, error)
	DeleteMomentPhoto(ctx context.Context, id uuid.UUID) error
	// MarkMomentVideo sets VideoAt for a video kept in the VideoStore.
	MarkMomentVideo(ctx context.Context, id uuid.UUID) error
	ClearMomentVideo(ctx context.Context, id uuid.UUID) error
}

// PhotoStore keeps photos outside the database (S3), under keys the service chooses. GetPhoto
// returns domain.ErrNotFound for a key it doesn't hold; deleting a missing key is no error.
type PhotoStore interface {
	PutPhoto(ctx context.Context, key string, p domain.Photo) error
	GetPhoto(ctx context.Context, key string) (domain.Photo, error)
	DeletePhoto(ctx context.Context, key string) error
}

// VideoStore keeps videos (S3), under keys the service chooses. They are streamed, never held in
// memory whole. StatVideo returns domain.ErrNotFound for a key it doesn't hold; deleting a missing
// key is no error.
type VideoStore interface {
	// PutVideo stores what r yields; an error from r aborts it and leaves nothing behind.
	PutVideo(ctx context.Context, key, contentType string, r io.Reader) error
	StatVideo(ctx context.Context, key string) (domain.Video, error)
	// OpenVideo reads length bytes from offset; closing it ends the read.
	OpenVideo(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
	DeleteVideo(ctx context.Context, key string) error
}

// JobRepository lets a daily job run once per day across all API instances.
type JobRepository interface {
	// ClaimJobRun returns true for the first caller with this job and day, false for every later one.
	ClaimJobRun(ctx context.Context, job string, day time.Time) (bool, error)
}

// RecipeBook is the household's recipe collection (Mealie). Every method returns
// domain.ErrUnavailable when the service can't be reached.
type RecipeBook interface {
	// Recipes returns the summaries of all recipes (up to a few hundred).
	Recipes(ctx context.Context) ([]domain.RecipeSummary, error)
	// Recipe returns domain.ErrNotFound for an unknown slug.
	Recipe(ctx context.Context, slug string) (domain.Recipe, error)
	// Image returns the recipe's picture; domain.ErrNotFound when it has none.
	Image(ctx context.Context, slug string) (domain.RecipeImage, error)
}

// MealPlanRepository keeps each household's dinners, one per day.
type MealPlanRepository interface {
	// SaveMeal replaces the household's dinner on m.Day.
	SaveMeal(ctx context.Context, m domain.PlannedMeal) error
	// Meals returns the dinners with from <= Day < to, by day.
	Meals(ctx context.Context, householdID uuid.UUID, from, to time.Time) ([]domain.PlannedMeal, error)
	// DeleteMeal removes the day's dinner; nothing planned is fine.
	DeleteMeal(ctx context.Context, householdID uuid.UUID, day time.Time) error
}

// ShoppingRepository keeps the household's shopping list. Calls that name an item return
// domain.ErrNotFound when it isn't on that household's list.
type ShoppingRepository interface {
	// ShoppingItems returns open items oldest first, then ticked ones, most recently ticked first.
	ShoppingItems(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.ShoppingItem, error)
	// AddShoppingItems stores the items, all or none.
	AddShoppingItems(ctx context.Context, items []domain.ShoppingItem) ([]domain.ShoppingItem, error)
	SetShoppingItemDone(ctx context.Context, householdID, id uuid.UUID, done bool) (domain.ShoppingItem, error)
	DeleteShoppingItem(ctx context.Context, householdID, id uuid.UUID) error
	ClearDoneShoppingItems(ctx context.Context, householdID uuid.UUID) error
	// ShoppingSuggestions returns what the household put on the list before, kept after items
	// are cleared, most used first, then most recent; texts open on the list are left out.
	ShoppingSuggestions(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.TitleSuggestion, error)
	SetShoppingItemAisle(ctx context.Context, householdID, id uuid.UUID, aisle domain.Aisle) (domain.ShoppingItem, error)
	// KnownAisles returns the remembered aisle of each of the keys (domain.AisleKey) it knows.
	KnownAisles(ctx context.Context, householdID uuid.UUID, keys []string) (map[string]domain.Aisle, error)
	// RememberAisles stores where the household puts these things, replacing what it knew.
	RememberAisles(ctx context.Context, householdID uuid.UUID, aisles map[string]domain.Aisle) error
}

// ChoreRepository keeps the household's chores and what was done. Calls that name a chore or a
// completion return domain.ErrNotFound when it isn't that household's.
type ChoreRepository interface {
	// Chores returns the chores still to do, soonest due first.
	Chores(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.Chore, error)
	CreateChore(ctx context.Context, c domain.Chore) (domain.Chore, error)
	GetChore(ctx context.Context, householdID, id uuid.UUID) (domain.Chore, error)
	// UpdateChore stores the chore's fields.
	UpdateChore(ctx context.Context, c domain.Chore) (domain.Chore, error)
	// DeleteChore deletes it; its completions stay, without it.
	DeleteChore(ctx context.Context, householdID, id uuid.UUID) error
	// CompleteChore stores the chore as after and the completion together, if the chore is still
	// as before (same due day and assignee, not done); else it returns domain.ErrConflict.
	CompleteChore(ctx context.Context, before, after domain.Chore, done domain.ChoreCompletion) (domain.ChoreCompletion, error)
	// LastChoreCompletion returns the chore's latest completion, or domain.ErrNotFound.
	LastChoreCompletion(ctx context.Context, householdID, choreID uuid.UUID) (domain.ChoreCompletion, error)
	GetChoreCompletion(ctx context.Context, householdID, id uuid.UUID) (domain.ChoreCompletion, error)
	// UndoChoreCompletion deletes the completion and puts its chore back as it was before, if the
	// completion is still the chore's latest; else it returns domain.ErrConflict.
	UndoChoreCompletion(ctx context.Context, done domain.ChoreCompletion) error
	// ChoreCompletions returns what was done on or after the day since, newest first.
	ChoreCompletions(ctx context.Context, householdID uuid.UUID, since time.Time, limit int) ([]domain.ChoreCompletion, error)
	// ThankChoreCompletion records the user's thanks; false when they had already said it.
	ThankChoreCompletion(ctx context.Context, householdID, id, userID uuid.UUID) (bool, error)
}

// BudgetRepository keeps the household's expenses and settlements. Calls that name an expense
// return domain.ErrNotFound when the household has none with that id.
type BudgetRepository interface {
	// Expenses returns those spent on days in [from, to), newest day first, with their shares.
	Expenses(ctx context.Context, householdID uuid.UUID, from, to time.Time, limit int) ([]domain.Expense, error)
	// LatestExpenseDay returns the newest day before `before` with an expense; domain.ErrNotFound when none.
	LatestExpenseDay(ctx context.Context, householdID uuid.UUID, before time.Time) (time.Time, error)
	Expense(ctx context.Context, householdID, id uuid.UUID) (domain.Expense, error)
	// CreateExpense and UpdateExpense store the expense with its shares, all or none.
	CreateExpense(ctx context.Context, e domain.Expense) (domain.Expense, error)
	UpdateExpense(ctx context.Context, e domain.Expense) (domain.Expense, error)
	DeleteExpense(ctx context.Context, householdID, id uuid.UUID) error
	// SetExpenseReceipt records when a receipt photo was kept in a PhotoStore; nil when it was removed.
	SetExpenseReceipt(ctx context.Context, householdID, id uuid.UUID, at *time.Time) (domain.Expense, error)
	// Nets returns, for everyone with an expense or share, what they paid minus their shares.
	Nets(ctx context.Context, householdID uuid.UUID) ([]domain.Net, error)
}

// Grocer is the LLM's sense of where things are in a shop.
type Grocer interface {
	// Aisles returns one aisle for each of the texts, in order. It returns domain.ErrUnavailable
	// when the LLM can't be asked.
	Aisles(ctx context.Context, texts []string) ([]domain.Aisle, error)
}

// Chef is the LLM's cooking judgement.
type Chef interface {
	// Pick chooses up to n of the candidates that fit the mood and says why, in the language
	// given as a BCP 47 tag. It returns domain.ErrUnavailable when the LLM can't be asked.
	Pick(ctx context.Context, mood, lang string, candidates []domain.RecipeSummary, n int) ([]domain.Pick, error)
	// Plan chooses n different dinners from the candidates, in the order to cook them, unlike the
	// recent ones (by name) and using up what is on hand. wish may be empty. Reasons are in the
	// language given as a BCP 47 tag. It returns domain.ErrUnavailable when the LLM can't be asked.
	Plan(ctx context.Context, wish, lang string, onHand, recent []string, candidates []domain.RecipeSummary, n int) ([]domain.Pick, error)
}

// Muse is the LLM writing questions for the household to answer together.
type Muse interface {
	// Questions writes up to n new questions in the language given as a BCP 47 tag, each in the
	// spirit of one of themes and unlike any of recent. It returns domain.ErrUnavailable when the
	// LLM can't be asked.
	Questions(ctx context.Context, lang string, themes, recent []string, n int) ([]string, error)
}

// Critic is the LLM's judgement on films and series.
type Critic interface {
	// Pick chooses up to n titles that fit the mood (which may be empty): mostly from candidates,
	// by id, and some from outside them, taking seen as the person's taste. Reasons are in the
	// language given as a BCP 47 tag. It returns domain.ErrUnavailable when the LLM can't be asked.
	Pick(ctx context.Context, mood, lang string, candidates, seen []domain.Title, n int) ([]domain.WatchPick, error)
}

// SmartHome is Home Assistant, narrowed to what Homeloom may do: read scenes, lights, a few
// sensors and the rooms, turn a scene on, switch a light. Locks, alarms, covers and other domains
// are deliberately not here.
// Calls act as the person who signed in, through the refresh token from that sign-in. Every
// method returns domain.ErrUnavailable when Home Assistant can't be reached, and
// domain.ErrUnauthenticated when it no longer accepts the refresh token.
type SmartHome interface {
	// AuthorizeURL is Home Assistant's sign-in page; it redirects back with a code and the state.
	AuthorizeURL(state string) string
	// Exchange redeems the code and returns the refresh token.
	Exchange(ctx context.Context, code string) (string, error)
	Revoke(ctx context.Context, refreshToken string) error
	Devices(ctx context.Context, refreshToken string) (domain.Devices, error)
	// Layout reads the rooms and which entities are in them or hidden.
	Layout(ctx context.Context, refreshToken string) (domain.Layout, error)
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

// MediaLibrary is Plex, narrowed to reading: the movie and series libraries on the servers a
// person owns or that friends share with them, their titles and posters. Nothing is played or
// changed. Calls act as the person who signed in, through the token from that sign-in. Every
// method returns domain.ErrUnavailable when Plex can't be reached, and domain.ErrUnauthenticated
// when it no longer accepts the token.
type MediaLibrary interface {
	// StartSignIn begins a sign-in at Plex. It returns Plex's id for it and the page to send the
	// browser to; once the person has approved Homeloom there, it comes back with the state.
	StartSignIn(ctx context.Context, state string) (pin, url string, err error)
	// FinishSignIn returns the token once the sign-in is approved, and domain.ErrUnauthenticated
	// when it wasn't.
	FinishSignIn(ctx context.Context, pin string) (string, error)
	Libraries(ctx context.Context, token string) ([]domain.Library, error)
	// Titles and Poster take ids that Libraries and Titles returned; one the person can't reach
	// is domain.ErrNotFound.
	Titles(ctx context.Context, token, libraryID string) ([]domain.Title, error)
	Poster(ctx context.Context, token, titleID string) (domain.Poster, error)
	// Title returns one title by an id Titles returned.
	Title(ctx context.Context, token, titleID string) (domain.Title, error)
}

// TonightRepository keeps each household's pick for tonight; saving replaces the last one.
type TonightRepository interface {
	SaveTonight(ctx context.Context, p domain.TonightPick) error
	// Tonight returns domain.ErrNotFound when nothing was picked.
	Tonight(ctx context.Context, householdID uuid.UUID) (domain.TonightPick, error)
	DeleteTonight(ctx context.Context, householdID uuid.UUID) error
}

type WatchConnectionRepository interface {
	// SaveWatchConnection replaces the person's connection.
	SaveWatchConnection(ctx context.Context, c domain.WatchConnection) error
	// WatchConnection returns domain.ErrNotFound when the person has none.
	WatchConnection(ctx context.Context, userID uuid.UUID) (domain.WatchConnection, error)
	// SetWatchLibraryHidden leaves a library out, or puts it back; domain.ErrNotFound without a
	// connection.
	SetWatchLibraryHidden(ctx context.Context, userID uuid.UUID, libraryID string, hidden bool) error
	DeleteWatchConnection(ctx context.Context, userID uuid.UUID) error
}

type PushSubscriptionRepository interface {
	// SavePushSubscription stores the subscription, replacing any with the same endpoint.
	SavePushSubscription(ctx context.Context, sub domain.PushSubscription) error
	PushSubscriptions(ctx context.Context, userIDs []uuid.UUID) ([]domain.PushSubscription, error)
	// DeletePushSubscription removes the user's subscription; a missing one is fine.
	DeletePushSubscription(ctx context.Context, userID uuid.UUID, endpoint string) error
	// DeletePushEndpoint removes a subscription the push service no longer knows, whoever owns it.
	DeletePushEndpoint(ctx context.Context, endpoint string) error
}

// Pusher delivers Web Push notifications (VAPID).
type Pusher interface {
	PublicKey() string
	// Send returns domain.ErrNotFound when the push service says the subscription is gone, and
	// domain.ErrUnavailable when it can't be reached.
	Send(ctx context.Context, sub domain.PushSubscription, n domain.Notification) error
}

// CallServer is LiveKit, which carries the household's voice and video calls.
type CallServer interface {
	// URL is where clients connect.
	URL() string
	// Token lets someone into the room to talk, listen and share video, under the given identity
	// and display name.
	Token(identity, name, room string) (string, error)
	// Participants returns the ids of the users in the room; none when it doesn't exist. It returns
	// domain.ErrUnavailable when LiveKit can't be reached.
	Participants(ctx context.Context, room string) ([]string, error)
	// Dispatch sends the named agent into the room with the given metadata. It returns
	// domain.ErrUnavailable when LiveKit can't be reached.
	Dispatch(ctx context.Context, room, agent, metadata string) error
}

// VoiceSettingsRepository keeps how Homeloom sounds for each member. VoiceSettings returns
// domain.ErrNotFound for a member who hasn't saved any.
type VoiceSettingsRepository interface {
	VoiceSettings(ctx context.Context, userID uuid.UUID) (domain.VoiceSettings, error)
	SaveVoiceSettings(ctx context.Context, userID uuid.UUID, v domain.VoiceSettings) error
}

// Speaker turns text into speech, the way the voice agent speaks on a call.
type Speaker interface {
	// Speak returns text spoken with v as MP3. It returns domain.ErrUnavailable when the speech
	// model can't be reached.
	Speak(ctx context.Context, text string, v domain.VoiceSettings) ([]byte, error)
}

// Assistant is the LLM as a chat partner that can use tools.
type Assistant interface {
	// Reply answers the last user message of history. It runs each tool the model asks for with
	// run, passes its text to emit as it is written, and returns the whole answer. It returns
	// domain.ErrUnavailable when the LLM can't be asked.
	Reply(ctx context.Context, brief domain.AssistantBrief, history []domain.ChatMessage, tools []domain.Tool, run ToolRunner, emit func(domain.ChatEvent)) (string, error)
}

// ToolRunner runs one tool call. Its result goes back to the model; an error is shown to the model
// as a failed call, so its message must be fit for that.
type ToolRunner func(ctx context.Context, call domain.ToolCall) (string, error)

type MCPTokenRepository interface {
	// CreateMCPToken stores the token under t.ID, or a new id when it is uuid.Nil.
	CreateMCPToken(ctx context.Context, tokenHash []byte, t domain.MCPToken) (domain.MCPToken, error)
	// ListMCPTokens returns the user's tokens in the household, newest first.
	ListMCPTokens(ctx context.Context, householdID, userID uuid.UUID) ([]domain.MCPToken, error)
	// DeleteMCPToken returns domain.ErrNotFound unless the user has a token with that id.
	DeleteMCPToken(ctx context.Context, id, userID uuid.UUID) error
	// MCPTokenByHash returns the token and its owner, or domain.ErrNotFound.
	MCPTokenByHash(ctx context.Context, tokenHash []byte) (domain.MCPToken, domain.User, error)
	// MCPTokenByID returns the token, its owner and its stored hash, or domain.ErrNotFound.
	MCPTokenByID(ctx context.Context, id uuid.UUID) (domain.MCPToken, domain.User, []byte, error)
}

type ConversationRepository interface {
	CreateConversation(ctx context.Context, c domain.Conversation) (domain.Conversation, error)
	// GetConversation returns domain.ErrNotFound for an unknown id.
	GetConversation(ctx context.Context, id uuid.UUID) (domain.Conversation, error)
	// ListConversations returns the person's conversations in the household, last used first.
	ListConversations(ctx context.Context, householdID, userID uuid.UUID, limit int) ([]domain.Conversation, error)
	DeleteConversation(ctx context.Context, id uuid.UUID) error
	// AddChatMessage stores the message and marks its conversation as just used.
	AddChatMessage(ctx context.Context, m domain.ChatMessage) (domain.ChatMessage, error)
	// ChatMessages returns the last limit messages of the conversation, oldest first.
	ChatMessages(ctx context.Context, conversationID uuid.UUID, limit int) ([]domain.ChatMessage, error)
}

type HealthCheck interface {
	Name() string
	Check(ctx context.Context) error
}
