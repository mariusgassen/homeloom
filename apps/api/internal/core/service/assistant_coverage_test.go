package service_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

// TestAssistantCoversEveryDrivingService guards against adding a feature to a driving service in
// port/driving.go without deciding whether the assistant and MCP server should offer it. Every
// method of every *Service interface in port/driving.go must be listed below with either the tool
// name(s) that expose it or "-: <reason>" explaining why it stays out of the tool whitelist.
func TestAssistantCoversEveryDrivingService(t *testing.T) {
	var assistant *service.Assistant
	knownTools := map[string]bool{}
	seenTools := map[string]bool{}
	for _, tool := range assistant.Tools() {
		if tool.Name == "" || tool.Description == "" || len(tool.Schema) == 0 {
			t.Errorf("tool %q is missing name, description or schema", tool.Name)
		}
		if knownTools[tool.Name] {
			t.Errorf("duplicate tool %q", tool.Name)
		}
		knownTools[tool.Name] = true
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "../port/driving.go", nil, 0)
	if err != nil {
		t.Fatalf("parse port/driving.go: %v", err)
	}

	methods := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || !strings.HasSuffix(ts.Name.Name, "Service") {
				continue
			}
			iface, ok := ts.Type.(*ast.InterfaceType)
			if !ok || iface.Methods == nil {
				continue
			}
			for _, field := range iface.Methods.List {
				for _, name := range field.Names {
					key := ts.Name.Name + "." + name.Name
					methods[key] = true
					spec, ok := serviceToolCoverage[key]
					if !ok {
						t.Errorf("%s is not in serviceToolCoverage: expose it as a tool in assistant_tools*.go or document why it is excluded", key)
						continue
					}
					if rest, excluded := strings.CutPrefix(spec, "-:"); excluded {
						if strings.TrimSpace(rest) == "" {
							t.Errorf("%s is excluded without a reason", key)
						}
						continue
					}
					for _, name := range strings.Split(spec, ",") {
						name = strings.TrimSpace(name)
						if !knownTools[name] {
							t.Errorf("%s references unknown tool %q", key, name)
						}
						seenTools[name] = true
					}
				}
			}
		}
	}

	for key := range serviceToolCoverage {
		if !methods[key] {
			t.Errorf("serviceToolCoverage has stale entry %s (no such method in port/driving.go)", key)
		}
	}
	for name := range knownTools {
		if !seenTools[name] {
			t.Errorf("tool %q is not referenced by any entry in serviceToolCoverage", name)
		}
	}
}

var serviceToolCoverage = map[string]string{
	// IdentityService — authentication transport, not a household action.
	"IdentityService.Authenticate":        "-: authentication transport",
	"IdentityService.AuthenticateSession": "-: authentication transport",

	// AvatarService — binary profile pictures managed in account settings.
	"AvatarService.Set":    "-: binary profile photo upload",
	"AvatarService.Delete": "-: assistant never deletes",
	"AvatarService.Get":    "-: binary profile photo download",

	// LoginService — browser OIDC sign-in flow.
	"LoginService.Begin":    "-: browser sign-in flow",
	"LoginService.Complete": "-: browser sign-in flow",
	"LoginService.Logout":   "-: browser session sign-out",

	// HouseholdService — reading members is a tool; membership & place setup stay in the app.
	"HouseholdService.Get":      "get_household",
	"HouseholdService.List":     "-: a token is already scoped to one household",
	"HouseholdService.Create":   "-: creating a household is an account action in the app",
	"HouseholdService.SetPlace": "-: household place setup lives in settings",
	"HouseholdService.Invite":   "-: inviting members is a security-sensitive action in the app",
	"HouseholdService.Join":     "-: joining a household is an account action in the app",

	// QuestionService — asking, answering and reading are tools; deleting/rewording/AI prompts aren't.
	"QuestionService.List":    "list_questions",
	"QuestionService.Ask":     "ask_question",
	"QuestionService.Answer":  "answer_question",
	"QuestionService.Reword":  "-: editing an asked question is done in the app",
	"QuestionService.Delete":  "-: assistant never deletes",
	"QuestionService.Suggest": "-: server-side LLM prompt helper; MCP clients bring their own model",

	// EntryService — reminders, events and notes.
	"EntryService.List":               "list_entries",
	"EntryService.Create":             "add_reminder, add_event, add_note",
	"EntryService.Get":                "update_entry",
	"EntryService.Update":             "update_entry",
	"EntryService.SetDone":            "complete_reminder, reopen_reminder",
	"EntryService.Suggestions":        "-: UI autocomplete helper",
	"EntryService.Delete":             "-: assistant never deletes",
	"EntryService.AddPhoto":           "-: binary photo upload",
	"EntryService.Photo":              "-: binary photo download",
	"EntryService.HasCalendarFeed":    "-: calendar feed credentials live in settings",
	"EntryService.CreateCalendarFeed": "-: calendar feed credentials live in settings",
	"EntryService.DeleteCalendarFeed": "-: assistant never deletes",
	"EntryService.CalendarFeed":       "-: public iCal feed endpoint",

	// MomentService — timeline moments, daily gratitude and emoji reactions.
	"MomentService.Month":       "list_moments",
	"MomentService.Create":      "add_moment, add_gratitude",
	"MomentService.React":       "react_to_moment",
	"MomentService.Get":         "-: moments are read by month via list_moments",
	"MomentService.Update":      "-: editing past moments is done in the app",
	"MomentService.Delete":      "-: assistant never deletes",
	"MomentService.Resurfaced":  "-: Today tab resurfacing widget",
	"MomentService.SetPhoto":    "-: binary photo upload",
	"MomentService.Photo":       "-: binary photo download",
	"MomentService.DeletePhoto": "-: assistant never deletes",
	"MomentService.SetVideo":    "-: binary video upload",
	"MomentService.Video":       "-: binary video stream",
	"MomentService.DeleteVideo": "-: assistant never deletes",
	"MomentService.Unreact":     "-: assistant never deletes",

	// EntrySyncService — CalDAV/WebDAV protocol adapter surface.
	"EntrySyncService.SyncEntries": "-: CalDAV/WebDAV sync protocol",
	"EntrySyncService.SyncEntry":   "-: CalDAV/WebDAV sync protocol",
	"EntrySyncService.SyncPut":     "-: CalDAV/WebDAV sync protocol",
	"EntrySyncService.SyncDelete":  "-: CalDAV/WebDAV sync protocol",
	"EntrySyncService.SyncMove":    "-: CalDAV/WebDAV sync protocol",

	// CookService — recipes, dinner plan and ratings.
	"CookService.Find":                   "search_recipes",
	"CookService.Recipe":                 "get_recipe",
	"CookService.Plan":                   "get_meal_plan",
	"CookService.PlanMeal":               "plan_meal",
	"CookService.Rate":                   "rate_meal",
	"CookService.Ratings":                "list_meal_ratings",
	"CookService.AddToShoppingList":      "-: add_to_shopping_list adds free-text items directly",
	"CookService.AddMealsToShoppingList": "-: add_to_shopping_list adds free-text items directly",
	"CookService.Suggest":                "-: server-side LLM suggestion; MCP clients search and pick directly",
	"CookService.SuggestPlan":            "-: server-side LLM suggestion; MCP clients plan directly",
	"CookService.Image":                  "-: binary recipe image",
	"CookService.UnplanMeal":             "-: assistant never deletes; plan_meal replaces a day's meal",
	"CookService.Unrate":                 "-: assistant never deletes; rate_meal replaces a rating",
	"CookService.Recap":                  "-: yearly UI recap view",

	// BudgetService — shared expenses, balance and settling up.
	"BudgetService.Month":         "get_money",
	"BudgetService.Balance":       "get_money",
	"BudgetService.Add":           "add_expense",
	"BudgetService.Settle":        "settle_up",
	"BudgetService.Update":        "-: editing past expenses is done in the app",
	"BudgetService.Delete":        "-: assistant never deletes",
	"BudgetService.SetReceipt":    "-: binary receipt photo upload",
	"BudgetService.Receipt":       "-: binary receipt photo download",
	"BudgetService.DeleteReceipt": "-: assistant never deletes",

	// ShoppingService — shared shopping list.
	"ShoppingService.Items":       "list_shopping",
	"ShoppingService.Add":         "add_to_shopping_list",
	"ShoppingService.SetDone":     "complete_shopping_item",
	"ShoppingService.AddItems":    "-: batch helper for recipe import; add_to_shopping_list covers tool callers",
	"ShoppingService.SetAisle":    "-: aisle sorting is an in-store UI gesture",
	"ShoppingService.Delete":      "-: assistant never deletes",
	"ShoppingService.ClearDone":   "-: assistant never deletes",
	"ShoppingService.Suggestions": "-: UI autocomplete helper",

	// DocumentService — binary file vault.
	"DocumentService.List":   "-: binary file vault stays in the app",
	"DocumentService.Add":    "-: binary file upload",
	"DocumentService.Update": "-: binary file vault stays in the app",
	"DocumentService.Delete": "-: assistant never deletes",
	"DocumentService.File":   "-: binary file download",

	// BucketService — someday list (watch, read, visit).
	"BucketService.List":    "list_bucket",
	"BucketService.Add":     "add_bucket_item",
	"BucketService.SetDone": "complete_bucket_item",
	"BucketService.Delete":  "-: assistant never deletes",

	// ChoreService — recurring & one-off chores, completions, fairness and thanks.
	"ChoreService.List":     "list_chores",
	"ChoreService.Create":   "add_chore",
	"ChoreService.Complete": "complete_chore",
	"ChoreService.History":  "get_chore_history",
	"ChoreService.Fairness": "get_chore_history",
	"ChoreService.Thank":    "thank_chore",
	"ChoreService.Update":   "-: editing chore setup is done in the app",
	"ChoreService.Delete":   "-: assistant never deletes",
	"ChoreService.Undo":     "-: assistant never deletes",

	// TripService — trips, options, votes and packing lists.
	"TripService.List":          "list_trips",
	"TripService.Create":        "add_trip",
	"TripService.AddOption":     "add_trip_option",
	"TripService.Vote":          "vote_trip_option",
	"TripService.AddPacking":    "add_packing_item",
	"TripService.UpdatePacking": "complete_packing_item",
	"TripService.Update":        "-: editing trip details is done in the app",
	"TripService.Delete":        "-: assistant never deletes",
	"TripService.UpdateOption":  "-: editing trip options is done in the app",
	"TripService.DeleteOption":  "-: assistant never deletes",
	"TripService.Unvote":        "-: assistant never deletes; vote_trip_option changes the vote",
	"TripService.AddPhoto":      "-: binary photo upload",
	"TripService.Photo":         "-: binary photo download",
	"TripService.DeletePhoto":   "-: assistant never deletes",
	"TripService.DeletePacking": "-: assistant never deletes",

	// WishService — shared household wishlist.
	"WishService.List":        "list_wishes",
	"WishService.Create":      "add_wish",
	"WishService.Vote":        "add_wish, vote_wish",
	"WishService.Got":         "complete_wish",
	"WishService.Update":      "-: editing wishes is done in the app",
	"WishService.Delete":      "-: assistant never deletes",
	"WishService.SetPhoto":    "-: binary photo upload",
	"WishService.Photo":       "-: binary photo download",
	"WishService.DeletePhoto": "-: assistant never deletes",

	// HomeService — Home Assistant rooms, scenes and lights.
	"HomeService.Get":             "get_home",
	"HomeService.ActivateScene":   "activate_scene",
	"HomeService.ToggleLight":     "set_light",
	"HomeService.Connect":         "-: browser OAuth flow",
	"HomeService.CompleteConnect": "-: browser OAuth flow",
	"HomeService.Disconnect":      "-: assistant never disconnects integrations",

	// WatchService — tonight's pick is readable; Plex OAuth & interactive swiping stay in the app.
	"WatchService.Tonight":         "get_tonight",
	"WatchService.Connect":         "-: browser OAuth flow",
	"WatchService.CompleteConnect": "-: browser OAuth flow",
	"WatchService.Disconnect":      "-: assistant never disconnects integrations",
	"WatchService.Shelf":           "-: Plex library management stays in the app",
	"WatchService.Include":         "-: Plex library management stays in the app",
	"WatchService.Titles":          "-: browsing Plex libraries stays in the app",
	"WatchService.Poster":          "-: binary poster image",
	"WatchService.Pick":            "-: picking from a Plex library uses in-app title ids",
	"WatchService.ClearTonight":    "-: assistant never deletes",
	"WatchService.Night":           "-: interactive two-person swipe game in the app",
	"WatchService.Swipe":           "-: interactive two-person swipe game in the app",
	"WatchService.ResetNight":      "-: assistant never deletes",
	"WatchService.Suggest":         "-: server-side LLM suggestion",

	// PushService — browser Web Push subscription per device.
	"PushService.PublicKey":   "-: browser Web Push setup",
	"PushService.Subscribe":   "-: browser Web Push setup",
	"PushService.Unsubscribe": "-: browser Web Push setup",
	"PushService.Topics":      "-: notification preferences live in settings",
	"PushService.SetTopic":    "-: notification preferences live in settings",

	// CallService — live WebRTC audio/video calls.
	"CallService.Start":          "-: live WebRTC call signalling",
	"CallService.Join":           "-: live WebRTC call signalling",
	"CallService.Invite":         "-: live WebRTC call signalling",
	"CallService.GuestLink":      "-: live WebRTC call signalling",
	"CallService.GuestPass":      "-: live WebRTC call signalling",
	"CallService.JoinAsGuest":    "-: live WebRTC call signalling",
	"CallService.TalkToHomeloom": "-: live WebRTC voice call entry point",

	// VoiceService — TTS voice settings for calls.
	"VoiceService.Available": "-: TTS voice configuration",
	"VoiceService.Settings":  "-: TTS voice configuration",
	"VoiceService.Save":      "-: TTS voice configuration",
	"VoiceService.Preview":   "-: binary audio preview",

	// AssistantService — the chat UI that drives the tools themselves.
	"AssistantService.Conversations":      "-: chat conversation history UI",
	"AssistantService.Conversation":       "-: chat conversation history UI",
	"AssistantService.DeleteConversation": "-: assistant never deletes",
	"AssistantService.Ask":                "-: chat turn orchestration",
	"AssistantService.Answer":             "-: chat turn orchestration",

	// ToolService — the meta-interface that exposes these tools.
	"ToolService.Tools":   "-: meta: lists these tools",
	"ToolService.RunTool": "-: meta: runs these tools",

	// AgentAccessService — minting and revoking MCP / CalDAV tokens is security-sensitive.
	"AgentAccessService.Mint":                 "-: token minting is security-sensitive",
	"AgentAccessService.Authenticate":         "-: authentication transport",
	"AgentAccessService.AuthenticatePassword": "-: authentication transport",
	"AgentAccessService.Revoke":               "-: token revocation lives in settings",
	"AgentAccessService.Tokens":               "-: token management lives in settings",
	"AgentAccessService.CreateToken":          "-: token creation lives in settings",
	"AgentAccessService.DeleteToken":          "-: token deletion lives in settings",

	// GiftService — private gift ideas for other members and birthdays.
	"GiftService.Overview":    "list_gift_ideas",
	"GiftService.Add":         "add_gift_idea",
	"GiftService.SetGiven":    "complete_gift_idea",
	"GiftService.SetBirthday": "set_birthday",
	"GiftService.Delete":      "-: assistant never deletes",

	// DateNightService — server-side LLM generator.
	"DateNightService.Ideas": "-: server-side LLM prompt helper; MCP clients bring their own model",

	// LocationService — device GPS reporting.
	"LocationService.Set":   "-: device GPS reporting",
	"LocationService.Clear": "-: device GPS reporting",

	// PlaceService — town/city typeahead in settings.
	"PlaceService.Search": "-: UI typeahead helper",
}
