package domain

import "github.com/google/uuid"

// PushSubscription is one device that gets Web Push notifications for its user.
type PushSubscription struct {
	UserID   uuid.UUID
	Endpoint string
	P256dh   string
	Auth     string
	// Lang is the BCP 47 tag the device's notifications are written in.
	Lang string
}

// Phrase is a notification argument that is itself a catalog message: the push adapter writes it in
// the device's language before filling it into the notification.
type Phrase struct {
	Key  string
	Args []any
}

// Notification is written by the push adapter from catalog keys "<Key>.title" and "<Key>.body",
// in each device's language, so the core never formats user-facing text itself.
type Notification struct {
	Key  string
	Args []any
	// URL is the app path to open when the notification is tapped.
	URL string
	// Tag lets a later notification replace an earlier one about the same thing.
	Tag string
}

// PushTopic is a kind of notification a member can switch off for themselves.
type PushTopic string

const (
	PushTopicReminders PushTopic = "reminders"
	PushTopicQuestions PushTopic = "questions"
	PushTopicCalls     PushTopic = "calls"
	PushTopicChores    PushTopic = "chores"
	PushTopicGifts     PushTopic = "gifts"
	PushTopicUs        PushTopic = "us"
	PushTopicWatch     PushTopic = "watch"
	PushTopicBriefing  PushTopic = "briefing"
)

// PushTopics lists every topic, in the order the app shows them.
var PushTopics = []PushTopic{
	PushTopicBriefing, PushTopicReminders, PushTopicQuestions, PushTopicChores,
	PushTopicGifts, PushTopicUs, PushTopicWatch, PushTopicCalls,
}

var pushTopicByKey = map[string]PushTopic{
	"push.reminder":           PushTopicReminders,
	"push.event":              PushTopicReminders,
	"push.event_allday":       PushTopicReminders,
	"push.event_later":        PushTopicReminders,
	"push.revealed":           PushTopicQuestions,
	"push.call":               PushTopicCalls,
	"push.chore_thanks":       PushTopicChores,
	"push.chores_due":         PushTopicChores,
	"push.gift_birthday":      PushTopicGifts,
	"push.gift_birthday_none": PushTopicGifts,
	"push.gratitude":          PushTopicUs,
	"push.gratitude_own":      PushTopicUs,
	"push.moment_reaction":    PushTopicUs,
	"push.on_this_day":        PushTopicUs,
	"push.last_month":         PushTopicUs,
	"push.watch_match":        PushTopicWatch,
	"push.watch_tonight":      PushTopicWatch,
	"push.briefing":           PushTopicBriefing,
}

// Topic is the topic a notification belongs to; "" means it is always sent.
func (n Notification) Topic() PushTopic { return pushTopicByKey[n.Key] }

// ValidPushTopic reports whether t is one of PushTopics.
func ValidPushTopic(t PushTopic) bool {
	for _, p := range PushTopics {
		if p == t {
			return true
		}
	}
	return false
}

// PushTopicSetting is whether a member gets notifications of one topic.
type PushTopicSetting struct {
	Topic   PushTopic
	Enabled bool
}
