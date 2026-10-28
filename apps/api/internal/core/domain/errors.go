package domain

import "errors"

var (
	// ErrNotFound also covers "exists but not visible to the caller", so callers cannot probe for ids.
	ErrNotFound        = errors.New("not found")
	ErrInvalid         = errors.New("invalid input")
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrConflict means the request is valid but the current state does not allow it.
	ErrConflict = errors.New("conflict")
	// ErrUnavailable means an outside service (Mealie, the LLM, ...) is down or not set up.
	ErrUnavailable = errors.New("unavailable")
)
