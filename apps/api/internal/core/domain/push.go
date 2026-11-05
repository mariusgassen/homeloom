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
