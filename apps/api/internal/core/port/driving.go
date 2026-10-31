package port

import (
	"context"
	"io"
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

// AvatarService keeps the pictures members set for themselves.
type AvatarService interface {
	Set(ctx context.Context, actor domain.User, p domain.Photo) (domain.User, error)
	Delete(ctx context.Context, actor domain.User) (domain.User, error)
	// Get returns domain.ErrNotFound unless the user is the actor or shares a household with
	// them, and has set a picture.
	Get(ctx context.Context, actor domain.User, userID uuid.UUID) (domain.Photo, error)
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
	// Reword and Delete are for whoever asked; others get domain.ErrForbidden.
	Reword(ctx context.Context, actor domain.User, questionID uuid.UUID, text string) (domain.Question, error)
	Delete(ctx context.Context, actor domain.User, questionID uuid.UUID) error
	// Suggest returns new questions to ask, written in lang; domain.ErrUnavailable without an LLM.
	Suggest(ctx context.Context, actor domain.User, householdID uuid.UUID, lang string) ([]string, error)
}

type EntryService interface {
	List(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.EntryKind) ([]domain.Entry, error)
	Create(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.EntryKind, f domain.EntryFields) (domain.Entry, error)
	Get(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Entry, error)
	// Suggestions returns titles of reminders the actor set before, to set again.
	Suggestions(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.TitleSuggestion, error)
	Update(ctx context.Context, actor domain.User, id uuid.UUID, f domain.EntryFields) (domain.Entry, error)
	Delete(ctx context.Context, actor domain.User, id uuid.UUID) error
	// SetDone ticks a reminder off (done true) or opens it again.
	SetDone(ctx context.Context, actor domain.User, id uuid.UUID, done bool) (domain.Entry, error)
	// AddPhoto stores a photo in a note the actor can change and returns its id. The note's text
	// links it; a photo the text no longer links goes with the note's next change.
	AddPhoto(ctx context.Context, actor domain.User, id uuid.UUID, p domain.Photo) (uuid.UUID, error)
	// Photo returns domain.ErrNotFound unless the actor can read the note the photo is in.
	Photo(ctx context.Context, actor domain.User, id, photoID uuid.UUID) (domain.Photo, error)
	// HasCalendarFeed says whether the actor has a calendar subscription link for the household.
	HasCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) (bool, error)
	// CreateCalendarFeed returns a new subscription link token; an earlier link stops working.
	CreateCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error)
	DeleteCalendarFeed(ctx context.Context, actor domain.User, householdID uuid.UUID) error
	// CalendarFeed returns the events and dated open reminders the token's owner can see, and the
	// household's time zone. It returns domain.ErrNotFound for an unknown token or a member who left.
	CalendarFeed(ctx context.Context, token string) ([]domain.Entry, *time.Location, error)
}

// MomentService is the household's timeline: moments, gratitude lines and what comes back from
// them.
type MomentService interface {
	// Month returns the moments of the month that day is in; the household's current month when
	// it is zero.
	Month(ctx context.Context, actor domain.User, householdID uuid.UUID, day time.Time) (domain.MomentMonth, error)
	// Create keeps a moment, or writes the actor's gratitude line for today, replacing an earlier one.
	Create(ctx context.Context, actor domain.User, householdID uuid.UUID, kind domain.MomentKind, f domain.MomentFields) (domain.Moment, error)
	Get(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Moment, error)
	Update(ctx context.Context, actor domain.User, id uuid.UUID, f domain.MomentFields) (domain.Moment, error)
	Delete(ctx context.Context, actor domain.User, id uuid.UUID) error
	// Resurfaced returns what the actor can see from this day in earlier years, and one older
	// gratitude line at random (nil when there is none).
	Resurfaced(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Moment, *domain.Moment, error)
	SetPhoto(ctx context.Context, actor domain.User, id uuid.UUID, p domain.Photo) (domain.Moment, error)
	// Photo returns domain.ErrNotFound unless the actor can see the moment and its photo.
	Photo(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Photo, error)
	DeletePhoto(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Moment, error)
	// SetVideo streams r in as the video of the actor's moment. It is domain.ErrUnavailable without
	// a VideoStore.
	SetVideo(ctx context.Context, actor domain.User, id uuid.UUID, contentType string, r io.Reader) (domain.Moment, error)
	// Video returns domain.ErrNotFound unless the actor can see the moment and its video.
	Video(ctx context.Context, actor domain.User, id uuid.UUID) (domain.VideoStream, error)
	DeleteVideo(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Moment, error)
}

// EntrySyncService lets a calendar app (over CalDAV) or a notes app (over WebDAV) read and change
// the events, reminders or notes its owner can see, one list at a time; "later" notes are in none.
// Entries are named as the app names them: by the name the app gave one it made, else by
// "<id>.ics".
type EntrySyncService interface {
	// SyncEntries returns the entries in the list: shared ones, or the actor's private ones.
	SyncEntries(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList) ([]domain.Entry, error)
	// SyncEntry returns domain.ErrNotFound unless the named entry is in the list.
	SyncEntry(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList, name string) (domain.Entry, error)
	// SyncPut makes or changes the named entry, putting it in the list, and says whether it is new.
	SyncPut(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList, name string, in domain.SyncEntry) (domain.Entry, bool, error)
	// SyncDelete deletes the named entry. One that has moved to another list is left alone.
	SyncDelete(ctx context.Context, actor domain.User, householdID uuid.UUID, l domain.SyncList, name string) error
	// SyncMove moves the named entry to another list of its kind under a new name.
	SyncMove(ctx context.Context, actor domain.User, householdID uuid.UUID, from domain.SyncList, name string, to domain.SyncList, newName string) (domain.Entry, error)
}

type CookService interface {
	// Suggest picks recipes for a mood; lang is the language the reasons are written in.
	Suggest(ctx context.Context, actor domain.User, householdID uuid.UUID, mood, lang string) (domain.Suggestions, error)
	Recipe(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string) (domain.Recipe, error)
	Image(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string) (domain.RecipeImage, error)
	// AddToShoppingList puts the recipe's ingredients with the given ids on the shopping list and
	// returns how many; unknown ids are ignored.
	AddToShoppingList(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string, ingredientIDs []string) (int, error)
	// Find returns up to n recipes whose name, tags or description hold the query's words, best
	// first; an empty query returns n at random.
	Find(ctx context.Context, actor domain.User, householdID uuid.UUID, query string, n int) ([]domain.RecipeSummary, error)
}

// BudgetService is the household's shared money: expenses split between members, where everyone
// stands, and settling up. Every member sees and changes everything in it.
type BudgetService interface {
	// Month returns one month of expenses and settlements; this month when month is zero.
	Month(ctx context.Context, actor domain.User, householdID uuid.UUID, month time.Time) (domain.ExpenseMonth, error)
	Add(ctx context.Context, actor domain.User, householdID uuid.UUID, f domain.ExpenseFields) (domain.Expense, error)
	// Update replaces an expense's fields; a settlement can't be changed (domain.ErrSettlement).
	Update(ctx context.Context, actor domain.User, householdID, id uuid.UUID, f domain.ExpenseFields) (domain.Expense, error)
	Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error
	// Settle records that from gave to the amount, on day (today when zero).
	Settle(ctx context.Context, actor domain.User, householdID, from, to uuid.UUID, cents int64, day time.Time) (domain.Expense, error)
	Balance(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Balance, error)
}

type ShoppingService interface {
	Items(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.ShoppingItem, error)
	// Add puts the texts on the list, noting the recipe they came from ("" for none).
	Add(ctx context.Context, actor domain.User, householdID uuid.UUID, texts []string, recipe string) ([]domain.ShoppingItem, error)
	SetDone(ctx context.Context, actor domain.User, householdID, id uuid.UUID, done bool) (domain.ShoppingItem, error)
	// SetAisle moves an item to another aisle; things with the same text go there from now on.
	SetAisle(ctx context.Context, actor domain.User, householdID, id uuid.UUID, aisle domain.Aisle) (domain.ShoppingItem, error)
	Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error
	// ClearDone removes the ticked items.
	ClearDone(ctx context.Context, actor domain.User, householdID uuid.UUID) error
	// Suggestions returns what was on the list before, to put on it again.
	Suggestions(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.TitleSuggestion, error)
}

// ChoreService is the household's chores: who does what, in turns, and who did how much.
type ChoreService interface {
	// List returns the chores still to do, soonest due first.
	List(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Chore, error)
	Create(ctx context.Context, actor domain.User, householdID uuid.UUID, f domain.ChoreFields) (domain.Chore, error)
	Update(ctx context.Context, actor domain.User, householdID, id uuid.UUID, f domain.ChoreFields) (domain.Chore, error)
	Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error
	// Complete ticks the chore off as done by the actor: it moves to its next day and, when it
	// rotates, to the next member.
	Complete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) (domain.ChoreCompletion, error)
	// Undo takes back the actor's own latest completion of a chore.
	Undo(ctx context.Context, actor domain.User, householdID, completionID uuid.UUID) error
	// Thank tells whoever did it thanks. With a line, it also becomes the actor's gratitude line
	// for today, unless they wrote one already; the line is returned when it was written.
	Thank(ctx context.Context, actor domain.User, householdID, completionID uuid.UUID, line string) (domain.ChoreCompletion, *domain.Moment, error)
	// History returns what was done in the last weeks, newest first.
	History(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.ChoreCompletion, error)
	Fairness(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.ChoreFairness, error)
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

type WatchService interface {
	// Connect starts the actor's sign-in at Plex and returns where to send the browser.
	Connect(ctx context.Context, actor domain.User, householdID uuid.UUID) (string, error)
	// CompleteConnect finishes it for the person who started it; it returns
	// domain.ErrUnauthenticated for an unknown state, someone else's sign-in, or one not approved.
	CompleteConnect(ctx context.Context, actor domain.User, state string) error
	Disconnect(ctx context.Context, actor domain.User, householdID uuid.UUID) error
	Shelf(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Shelf, error)
	// Include puts a library on the actor's shelf and in ideas, or leaves it out.
	Include(ctx context.Context, actor domain.User, householdID uuid.UUID, libraryID string, included bool) error
	Titles(ctx context.Context, actor domain.User, householdID uuid.UUID, libraryID string) ([]domain.Title, error)
	Poster(ctx context.Context, actor domain.User, householdID uuid.UUID, titleID string) (domain.Poster, error)
	// Pick makes one of the actor's titles the household's pick for tonight and tells the others.
	Pick(ctx context.Context, actor domain.User, householdID uuid.UUID, titleID string) (domain.TonightPick, error)
	// Tonight returns nil when nothing is picked for tonight.
	Tonight(ctx context.Context, actor domain.User, householdID uuid.UUID) (*domain.TonightPick, error)
	ClearTonight(ctx context.Context, actor domain.User, householdID uuid.UUID) error
	// Suggest returns ideas that fit the mood, from the actor's libraries when connected and from
	// outside them; domain.ErrUnavailable without an LLM.
	Suggest(ctx context.Context, actor domain.User, householdID uuid.UUID, mood, lang string) ([]domain.WatchIdea, error)
}

type PushService interface {
	// PublicKey returns domain.ErrUnavailable when push isn't set up.
	PublicKey(ctx context.Context) (string, error)
	// Subscribe stores the actor's device; lang is the language its notifications are written in.
	Subscribe(ctx context.Context, actor domain.User, sub domain.PushSubscription) error
	Unsubscribe(ctx context.Context, actor domain.User, endpoint string) error
}

type CallService interface {
	// Start opens a new call in the household and rings the members in invite. It returns
	// domain.ErrNotFound when one of them isn't a member, domain.ErrInvalid for the actor
	// themselves, and domain.ErrUnavailable when calls aren't set up or LiveKit is down.
	Start(ctx context.Context, actor domain.User, householdID uuid.UUID, invite []uuid.UUID) (domain.Call, error)
	// Join lets the actor into one of the household's calls, e.g. to answer it.
	Join(ctx context.Context, actor domain.User, householdID, callID uuid.UUID) (domain.Call, error)
	// Invite rings more members into a call; those already in it aren't rung.
	Invite(ctx context.Context, actor domain.User, householdID, callID uuid.UUID, userIDs []uuid.UUID) error
	// GuestLink makes a link that lets someone without an account into the call.
	GuestLink(ctx context.Context, actor domain.User, householdID, callID uuid.UUID) (domain.GuestLink, error)
	// GuestPass returns domain.ErrNotFound for an unknown or expired guest link.
	GuestPass(ctx context.Context, token string) (domain.GuestPass, error)
	// JoinAsGuest lets the link's holder into its call under the name they give. The first device
	// to use a link claims it; another one gets domain.ErrConflict.
	JoinAsGuest(ctx context.Context, token, device, name string) (domain.Call, error)
	// TalkToHomeloom starts a private call between the actor and the Homeloom voice agent, which
	// acts as the actor and speaks lang. It returns domain.ErrUnavailable when the agent isn't set
	// up.
	TalkToHomeloom(ctx context.Context, actor domain.User, householdID uuid.UUID, lang string) (domain.Call, error)
}

// VoiceService is how Homeloom sounds on each member's own calls with it.
type VoiceService interface {
	// Available says whether Homeloom can speak at all: calls with it and previews.
	Available() bool
	// Settings returns the actor's settings, or domain.DefaultVoice until they save their own.
	Settings(ctx context.Context, actor domain.User) (domain.VoiceSettings, error)
	Save(ctx context.Context, actor domain.User, v domain.VoiceSettings) (domain.VoiceSettings, error)
	// Preview speaks text with v, which needn't be saved. It returns domain.ErrUnavailable when
	// speech isn't set up or can't be reached.
	Preview(ctx context.Context, actor domain.User, v domain.VoiceSettings, text string) ([]byte, error)
}

// AssistantService is the chat assistant. Each conversation belongs to one person, and the
// assistant acts only as that person, with the same rules as every other service.
type AssistantService interface {
	Conversations(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.Conversation, error)
	Conversation(ctx context.Context, actor domain.User, id uuid.UUID) (domain.Conversation, []domain.ChatMessage, error)
	DeleteConversation(ctx context.Context, actor domain.User, id uuid.UUID) error
	// Ask stores the actor's message, in a new conversation when conversationID is nil. It returns
	// domain.ErrUnavailable when the assistant isn't set up, before anything is stored.
	Ask(ctx context.Context, actor domain.User, householdID uuid.UUID, conversationID *uuid.UUID, text string) (domain.Conversation, domain.ChatMessage, error)
	// Answer replies to the conversation's last message in the language lang, streaming through
	// emit, and stores the reply.
	Answer(ctx context.Context, actor domain.User, conversationID uuid.UUID, lang string, emit func(domain.ChatEvent)) (domain.ChatMessage, error)
}

// ToolService offers the assistant's tools to other clients, such as the MCP server.
type ToolService interface {
	Tools() []domain.Tool
	// RunTool runs a tool as actor in the household, with the same rules as the assistant. Its
	// error is text meant for the model that called it.
	RunTool(ctx context.Context, actor domain.User, householdID uuid.UUID, call domain.ToolCall) (string, error)
}

// AgentAccessService hands out tokens with which another app or a helper, such as the voice
// agent, uses the tools as one member of one household.
type AgentAccessService interface {
	// Mint returns a token for actor in the household that lasts ttl (at most
	// domain.MaxAgentTokenTTL).
	Mint(ctx context.Context, actor domain.User, householdID uuid.UUID, ttl time.Duration) (string, error)
	// Authenticate takes a minted or a personal token for the tools. It returns
	// domain.ErrUnauthenticated for an unknown, revoked or expired one.
	Authenticate(ctx context.Context, token string) (domain.AgentGrant, error)
	// AuthenticatePassword takes a personal password for calendar or notes apps, which opens only
	// that access. It returns domain.ErrUnauthenticated for anything else.
	AuthenticatePassword(ctx context.Context, access domain.TokenAccess, password string) (domain.AgentGrant, error)
	Revoke(ctx context.Context, token string) error
	// Tokens are the actor's personal tokens in the household.
	Tokens(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.MCPToken, error)
	// CreateToken makes a personal token for the access that lasts until it is deleted, and
	// returns its secret.
	CreateToken(ctx context.Context, actor domain.User, householdID uuid.UUID, name string, access domain.TokenAccess) (domain.MCPToken, string, error)
	DeleteToken(ctx context.Context, actor domain.User, id uuid.UUID) error
}
