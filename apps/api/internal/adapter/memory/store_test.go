package memory_test

import (
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/storetest"
)

func TestInvites(t *testing.T)   { storetest.Invites(t, memory.NewStore()) }
func TestQuestions(t *testing.T) { storetest.Questions(t, memory.NewStore()) }
