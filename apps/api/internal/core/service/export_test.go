package service

import "time"

// SetClock lets tests move time for an Entry service.
func (s *Entry) SetClock(now func() time.Time) { s.now = now }
