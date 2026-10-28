package domain

import "errors"

var (
	// ErrNotFound also covers "exists but not visible to the caller", so callers cannot probe for ids.
	ErrNotFound        = errors.New("not found")
	ErrInvalid         = errors.New("invalid input")
	ErrUnauthenticated = errors.New("unauthenticated")
)
