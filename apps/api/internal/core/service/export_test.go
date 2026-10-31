package service

import "time"

// SetClock lets tests move time for an Entry service.
func (s *Entry) SetClock(now func() time.Time) { s.now = now }

// SetClock lets tests move time for an AgentAccess service.
func (s *AgentAccess) SetClock(now func() time.Time) { s.now = now }

// SetClock lets tests move time for a Moments service.
func (s *Moments) SetClock(now func() time.Time) { s.now = now }

// SetChance lets tests decide whether a past gratitude line comes back: below 1/3 it does.
func (s *Moments) SetChance(chance func() float64) { s.chance = chance }

// SetClock lets tests move time for a Watch service.
func (s *Watch) SetClock(now func() time.Time) { s.now = now }

// SetClock lets tests move time for a Chores service.
func (s *Chores) SetClock(now func() time.Time) { s.now = now }
