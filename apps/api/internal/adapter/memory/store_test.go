package memory_test

import (
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/storetest"
)

func TestInvites(t *testing.T)             { storetest.Invites(t, memory.NewStore()) }
func TestAvatars(t *testing.T)             { storetest.Avatars(t, memory.NewStore()) }
func TestQuestions(t *testing.T)           { storetest.Questions(t, memory.NewStore()) }
func TestHomeConnections(t *testing.T)     { storetest.HomeConnections(t, memory.NewStore()) }
func TestWatchConnections(t *testing.T)    { storetest.WatchConnections(t, memory.NewStore()) }
func TestTonight(t *testing.T)             { storetest.Tonight(t, memory.NewStore()) }
func TestPushSubscriptions(t *testing.T)   { storetest.PushSubscriptions(t, memory.NewStore()) }
func TestEntries(t *testing.T)             { storetest.Entries(t, memory.NewStore()) }
func TestCalendarFeeds(t *testing.T)       { storetest.CalendarFeeds(t, memory.NewStore()) }
func TestShoppingItems(t *testing.T)       { storetest.ShoppingItems(t, memory.NewStore()) }
func TestExpenses(t *testing.T)            { storetest.Expenses(t, memory.NewStore()) }
func TestShoppingSuggestions(t *testing.T) { storetest.ShoppingSuggestions(t, memory.NewStore()) }
func TestReminderSuggestions(t *testing.T) { storetest.ReminderSuggestions(t, memory.NewStore()) }
func TestConversations(t *testing.T)       { storetest.Conversations(t, memory.NewStore()) }
func TestMCPTokens(t *testing.T)           { storetest.MCPTokens(t, memory.NewStore()) }
func TestMoments(t *testing.T)             { storetest.Moments(t, memory.NewStore()) }
func TestVoiceSettings(t *testing.T)       { storetest.VoiceSettings(t, memory.NewStore()) }
