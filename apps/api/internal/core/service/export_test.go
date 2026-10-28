package service

import "time"

// SetClock lets tests move time for an Entry service.
func (s *Entry) SetClock(now func() time.Time) { s.now = now }

// SetClock lets tests move time for an AgentAccess service.
func (s *AgentAccess) SetClock(now func() time.Time) { s.now = now }
