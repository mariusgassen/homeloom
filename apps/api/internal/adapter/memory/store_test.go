package memory_test

import (
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/storetest"
)

func TestInvites(t *testing.T)           { storetest.Invites(t, memory.NewStore()) }
func TestQuestions(t *testing.T)         { storetest.Questions(t, memory.NewStore()) }
func TestHomeConnections(t *testing.T)   { storetest.HomeConnections(t, memory.NewStore()) }
func TestPushSubscriptions(t *testing.T) { storetest.PushSubscriptions(t, memory.NewStore()) }
