package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// More of the assistant's whitelist, kept apart from the first tools only for size. They follow the
// same rules: nothing deletes, every call goes through the same service the app uses with the
// person as the actor, and a part of the app that isn't wired answers domain.ErrUnavailable.
//
// Every method of a service in port/driving.go is either behind one of the tools or listed with a
// reason in assistant_coverage_test.go, so a new feature can't quietly stay out of reach of MCP.
var moreTools = []domain.Tool{
	{
		Name:        "get_household",
		ReadOnly:    true,
		Description: "Read who is in the household: the members' names, and which one is the person asking. Other tools take members by these names.",
		Schema:      object(map[string]any{}),
	},
	{
		Name: "update_entry",
		Description: "Change a reminder, event or note by its id from list_entries; what is left out stays as it is. Times are local, " +
			"YYYY-MM-DDTHH:MM (or YYYY-MM-DD); an empty string clears a time. A note that is still sealed can't be changed.",
		Schema: object(map[string]any{
			"id":      str("The entry's id."),
			"text":    str("The new text."),
			"due":     str("A reminder's new due time."),
			"starts":  str("An event's new start."),
			"ends":    str("An event's new end."),
			"all_day": map[string]any{"type": "boolean"},
			"repeat":  repeatEnum,
		}, "id"),
	},
	{
		Name:        "reopen_reminder",
		Description: "Open a reminder that was ticked off again, by its id from list_entries.",
		Schema:      object(map[string]any{"id": str("The reminder's id.")}, "id"),
	},
	{
		Name:        "complete_shopping_item",
		Description: "Tick an item on the shopping list off (done defaults to true; false puts it back), by its id from list_shopping.",
		Schema:      object(map[string]any{"id": str("The item's id."), "done": map[string]any{"type": "boolean"}}, "id"),
	},
	{
		Name:        "get_meal_plan",
		ReadOnly:    true,
		Description: "Read the household's dinner plan: which recipe is planned for which day. from is YYYY-MM-DD (today when left out); days defaults to 7.",
		Schema: object(map[string]any{
			"from": str("First day, YYYY-MM-DD."),
			"days": map[string]any{"type": "integer", "minimum": 1, "maximum": maxPlanDays},
		}),
	},
	{
		Name: "plan_meal",
		Description: "Make a recipe the dinner of a day, replacing what was planned, by the recipe's slug from search_recipes. " +
			"day is YYYY-MM-DD.",
		Schema: object(map[string]any{"day": str("The day, YYYY-MM-DD."), "slug": str("The recipe's slug.")}, "day", "slug"),
	},
	{
		Name: "rate_meal",
		Description: "Say how the person likes a dish, by the recipe's slug: love, fine, or nope (not again; it is left out of ideas). " +
			"It replaces the person's earlier rating.",
		Schema: object(map[string]any{"slug": str("The recipe's slug."), "rating": enum(string(domain.RatingLove), string(domain.RatingFine), string(domain.RatingNope))}, "slug", "rating"),
	},
	{
		Name:        "list_meal_ratings",
		ReadOnly:    true,
		Description: "List how each member rated the dishes they have eaten.",
		Schema:      object(map[string]any{}),
	},
	{
		Name:        "list_bucket",
		ReadOnly:    true,
		Description: "List the household's someday list of things to watch, read or visit; open ones first, ticked ones last.",
		Schema:      object(map[string]any{"kind": enum(string(domain.BucketWatch), string(domain.BucketRead), string(domain.BucketVisit))}),
	},
	{
		Name:        "add_bucket_item",
		Description: "Put something on the someday list: to watch, read or visit.",
		Schema: object(map[string]any{
			"kind": enum(string(domain.BucketWatch), string(domain.BucketRead), string(domain.BucketVisit)),
			"text": str("What it is, at most 200 characters."),
		}, "kind", "text"),
	},
	{
		Name:        "complete_bucket_item",
		Description: "Tick a someday item off (watched, read, been there; done defaults to true, false opens it again), by its id from list_bucket.",
		Schema:      object(map[string]any{"id": str("The item's id."), "done": map[string]any{"type": "boolean"}}, "id"),
	},
	{
		Name:        "list_trips",
		ReadOnly:    true,
		Description: "List the household's trips with the places they are weighing up (and who favours which) and their packing lists, soonest first.",
		Schema:      object(map[string]any{}),
	},
	{
		Name:        "add_trip",
		Description: "Plan a new trip. start and end are YYYY-MM-DD and optional.",
		Schema: object(map[string]any{
			"name":        str("The trip's name, at most 80 characters."),
			"destination": str("Where to."),
			"start":       str("First day."),
			"end":         str("Last day."),
			"notes":       str("Notes, at most 1000 characters."),
		}, "name"),
	},
	{
		Name:        "add_trip_option",
		Description: "Add a place or stay the household is weighing up for a trip, by the trip's id from list_trips.",
		Schema: object(map[string]any{
			"trip_id": str("The trip's id."),
			"title":   str("What it is, at most 100 characters."),
			"url":     str("Web link (http or https)."),
			"notes":   str("Notes."),
		}, "trip_id", "title"),
	},
	{
		Name:        "vote_trip_option",
		Description: "Make a place the person's favourite for a trip, by the ids from list_trips.",
		Schema:      object(map[string]any{"trip_id": str("The trip's id."), "option_id": str("The option's id.")}, "trip_id", "option_id"),
	},
	{
		Name:        "add_packing_item",
		Description: "Add something to a trip's packing list, by the trip's id from list_trips.",
		Schema:      object(map[string]any{"trip_id": str("The trip's id."), "text": str("What to pack, at most 100 characters.")}, "trip_id", "text"),
	},
	{
		Name:        "complete_packing_item",
		Description: "Tick a packing item off (packed; done defaults to true, false unpacks it), by the ids from list_trips.",
		Schema: object(map[string]any{
			"trip_id": str("The trip's id."), "item_id": str("The item's id."), "done": map[string]any{"type": "boolean"},
		}, "trip_id", "item_id"),
	},
	{
		Name:     "list_moments",
		ReadOnly: true,
		Description: "List the moments and gratitude lines of a month that the person can see, newest first. month is YYYY-MM, this month when left out. " +
			"A moment that opens later shows only that it exists.",
		Schema: object(map[string]any{"month": str("The month, YYYY-MM.")}),
	},
	{
		Name: "add_moment",
		Description: "Keep a moment of the household's story, such as a day out or something funny. day is YYYY-MM-DD, today when left out, " +
			"never later than today. visibility: private (only the person), shared, or reveal (the others see it from reveal_at).",
		Schema: object(map[string]any{
			"text":       str("The moment, at most 2000 characters."),
			"visibility": visibilityEnum("private", "shared", "reveal"),
			"day":        str("The day it happened."),
			"reveal_at":  str("When a reveal moment opens, YYYY-MM-DDTHH:MM."),
		}, "text", "visibility"),
	},
	{
		Name:        "add_gratitude",
		Description: "Write down what the person is grateful for today, shared with the household. It replaces the line they wrote earlier today.",
		Schema:      object(map[string]any{"text": str("At most 200 characters.")}, "text"),
	},
	{
		Name:        "react_to_moment",
		Description: "React to a moment with one emoji, by its id from list_moments. It replaces the person's earlier reaction.",
		Schema:      object(map[string]any{"id": str("The moment's id."), "emoji": enum(domain.ReactionEmojis...)}, "id", "emoji"),
	},
	{
		Name:     "list_questions",
		ReadOnly: true,
		Description: "List the questions the members asked each other, newest first. An answer stays sealed until everyone answered, " +
			"so only the person's own and revealed answers show.",
		Schema: object(map[string]any{}),
	},
	{
		Name:        "ask_question",
		Description: "Ask the other members a question to learn about each other, at most 280 characters.",
		Schema:      object(map[string]any{"text": str("The question.")}, "text"),
	},
	{
		Name:        "answer_question",
		Description: "Answer a question by its id from list_questions, at most 2000 characters.",
		Schema:      object(map[string]any{"id": str("The question's id."), "text": str("The answer.")}, "id", "text"),
	},
	{
		Name:        "settle_up",
		Description: "Record that one member paid another back, in euros, to even out the household's money. from is the member who paid back (the person when left out); day is YYYY-MM-DD, today when left out.",
		Schema: object(map[string]any{
			"to":     str("Name of the member who got the money."),
			"amount": map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": domain.MaxExpenseCents / 100, "description": "Euros."},
			"from":   str("Name of the member who paid back."),
			"day":    str("The day, YYYY-MM-DD."),
		}, "to", "amount"),
	},
	{
		Name:        "complete_gift_idea",
		Description: "Mark one of the person's own gift ideas as given (given defaults to true; false makes it an idea again), by its id from list_gift_ideas.",
		Schema:      object(map[string]any{"id": str("The idea's id."), "given": map[string]any{"type": "boolean"}}, "id"),
	},
	{
		Name:        "set_birthday",
		Description: "Set the person's own birthday, so the others can plan gifts. year is the year of birth and optional.",
		Schema: object(map[string]any{
			"month": map[string]any{"type": "integer", "minimum": 1, "maximum": 12},
			"day":   map[string]any{"type": "integer", "minimum": 1, "maximum": 31},
			"year":  map[string]any{"type": "integer"},
		}, "month", "day"),
	},
	{
		Name:        "get_chore_history",
		ReadOnly:    true,
		Description: "Read what was done lately around the house, newest first, and how many points each member earned this week and this month.",
		Schema:      object(map[string]any{}),
	},
	{
		Name:        "thank_chore",
		Description: "Say thanks to a member for a chore they did, by the completion's id from get_chore_history. A line, if given, also becomes the person's gratitude line for today.",
		Schema:      object(map[string]any{"id": str("The completion's id."), "line": str("What to say, at most 200 characters.")}, "id"),
	},
	{
		Name:        "vote_wish",
		Description: "Say how much the person wants a wish, from 1 to 3 hearts (0 takes their hearts back), by its id from list_wishes.",
		Schema: object(map[string]any{
			"id": str("The wish's id."), "hearts": map[string]any{"type": "integer", "minimum": 0, "maximum": domain.MaxWishHearts},
		}, "id", "hearts"),
	},
	{
		Name:        "complete_wish",
		Description: "Mark a wish as come true (got defaults to true; false makes it a wish again), by its id from list_wishes.",
		Schema:      object(map[string]any{"id": str("The wish's id."), "got": map[string]any{"type": "boolean"}}, "id"),
	},
	{
		Name:        "get_tonight",
		ReadOnly:    true,
		Description: "Read what the household picked to watch tonight, if anything.",
		Schema:      object(map[string]any{}),
	},
}

// maxPlanDays is how many days of the dinner plan one call returns.
const maxPlanDays = 31

// callMore runs the tools of moreTools; it is where call ends for any other name.
func (t *toolbox) callMore(ctx context.Context, call domain.ToolCall) (any, error) {
	switch call.Name {
	case "get_household":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		members := make([]string, len(t.members))
		for i, m := range t.members {
			members[i] = t.name(m.User.ID)
		}
		return map[string]any{"members": members, "you": t.actor.Name}, nil
	case "update_entry":
		var in struct {
			ID     uuid.UUID      `json:"id"`
			Text   *string        `json:"text"`
			Due    *string        `json:"due"`
			Starts *string        `json:"starts"`
			Ends   *string        `json:"ends"`
			AllDay *bool          `json:"all_day"`
			Repeat *domain.Repeat `json:"repeat"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.updateEntry(ctx, in.ID, in.Text, in.Due, in.Starts, in.Ends, in.AllDay, in.Repeat)
	case "reopen_reminder":
		var in struct {
			ID uuid.UUID `json:"id"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		e, err := t.s.entries.SetDone(ctx, t.actor, in.ID, false)
		if err != nil {
			return nil, err
		}
		return t.view(e), nil
	case "complete_shopping_item":
		var in struct {
			ID   uuid.UUID `json:"id"`
			Done *bool     `json:"done"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		it, err := t.s.shopping.SetDone(ctx, t.actor, t.householdID, in.ID, flag(in.Done))
		if err != nil {
			return nil, err
		}
		return shoppingView([]domain.ShoppingItem{it})[0], nil
	case "get_meal_plan":
		var in struct {
			From string `json:"from"`
			Days int    `json:"days"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.mealPlan(ctx, in.From, in.Days)
	case "plan_meal":
		var in struct {
			Day  string `json:"day"`
			Slug string `json:"slug"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		day, err := t.date("day", in.Day)
		if err != nil {
			return nil, err
		}
		if day.IsZero() {
			return nil, fmt.Errorf("%w: day is needed", domain.ErrInvalid)
		}
		m, err := t.s.cook.PlanMeal(ctx, t.actor, t.householdID, day, in.Slug)
		if err != nil {
			return nil, err
		}
		return t.plannedView(m), nil
	case "rate_meal":
		var in struct {
			Slug   string        `json:"slug"`
			Rating domain.Rating `json:"rating"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		r, err := t.s.cook.Rate(ctx, t.actor, t.householdID, in.Slug, in.Rating)
		if err != nil {
			return nil, err
		}
		return t.ratingView(r), nil
	case "list_meal_ratings":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		rs, err := t.s.cook.Ratings(ctx, t.actor, t.householdID)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(rs))
		for i, r := range rs {
			out[i] = t.ratingView(r)
		}
		return out, nil
	case "list_bucket":
		var in struct {
			Kind domain.BucketKind `json:"kind"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.bucket == nil {
			return nil, domain.ErrUnavailable
		}
		items, err := t.s.bucket.List(ctx, t.actor, t.householdID, in.Kind)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(items))
		for i, it := range items {
			out[i] = bucketView(it)
		}
		return out, nil
	case "add_bucket_item":
		var in struct {
			Kind domain.BucketKind `json:"kind"`
			Text string            `json:"text"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.bucket == nil {
			return nil, domain.ErrUnavailable
		}
		it, err := t.s.bucket.Add(ctx, t.actor, t.householdID, in.Kind, in.Text)
		if err != nil {
			return nil, err
		}
		return bucketView(it), nil
	case "complete_bucket_item":
		var in struct {
			ID   uuid.UUID `json:"id"`
			Done *bool     `json:"done"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.bucket == nil {
			return nil, domain.ErrUnavailable
		}
		it, err := t.s.bucket.SetDone(ctx, t.actor, t.householdID, in.ID, flag(in.Done))
		if err != nil {
			return nil, err
		}
		return bucketView(it), nil
	case "list_trips":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		if t.s.trips == nil {
			return nil, domain.ErrUnavailable
		}
		trips, err := t.s.trips.List(ctx, t.actor, t.householdID)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(trips))
		for i, tr := range trips {
			out[i] = t.tripView(tr)
		}
		return out, nil
	case "add_trip":
		var in struct {
			Name        string `json:"name"`
			Destination string `json:"destination"`
			Start       string `json:"start"`
			End         string `json:"end"`
			Notes       string `json:"notes"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.addTrip(ctx, in.Name, in.Destination, in.Start, in.End, in.Notes)
	case "add_trip_option":
		var in struct {
			TripID uuid.UUID `json:"trip_id"`
			Title  string    `json:"title"`
			URL    string    `json:"url"`
			Notes  string    `json:"notes"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.trips == nil {
			return nil, domain.ErrUnavailable
		}
		tr, err := t.s.trips.AddOption(ctx, t.actor, t.householdID, in.TripID, domain.TripOptionFields{Title: in.Title, URL: in.URL, Notes: in.Notes})
		if err != nil {
			return nil, err
		}
		return t.tripView(tr), nil
	case "vote_trip_option":
		var in struct {
			TripID   uuid.UUID `json:"trip_id"`
			OptionID uuid.UUID `json:"option_id"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.trips == nil {
			return nil, domain.ErrUnavailable
		}
		tr, err := t.s.trips.Vote(ctx, t.actor, t.householdID, in.TripID, in.OptionID)
		if err != nil {
			return nil, err
		}
		return t.tripView(tr), nil
	case "add_packing_item":
		var in struct {
			TripID uuid.UUID `json:"trip_id"`
			Text   string    `json:"text"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.trips == nil {
			return nil, domain.ErrUnavailable
		}
		tr, err := t.s.trips.AddPacking(ctx, t.actor, t.householdID, in.TripID, in.Text)
		if err != nil {
			return nil, err
		}
		return t.tripView(tr), nil
	case "complete_packing_item":
		var in struct {
			TripID uuid.UUID `json:"trip_id"`
			ItemID uuid.UUID `json:"item_id"`
			Done   *bool     `json:"done"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.packItem(ctx, in.TripID, in.ItemID, flag(in.Done))
	case "list_moments":
		var in struct {
			Month string `json:"month"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.moments(ctx, in.Month)
	case "add_moment":
		var in struct {
			Text       string            `json:"text"`
			Visibility domain.Visibility `json:"visibility"`
			Day        string            `json:"day"`
			RevealAt   string            `json:"reveal_at"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.addMoment(ctx, domain.KindMoment, in.Text, in.Visibility, in.Day, in.RevealAt)
	case "add_gratitude":
		var in struct {
			Text string `json:"text"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.addMoment(ctx, domain.KindGratitude, in.Text, domain.VisibilityShared, "", "")
	case "react_to_moment":
		var in struct {
			ID    uuid.UUID `json:"id"`
			Emoji string    `json:"emoji"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.momentsSvc == nil {
			return nil, domain.ErrUnavailable
		}
		m, err := t.s.momentsSvc.React(ctx, t.actor, in.ID, in.Emoji)
		if err != nil {
			return nil, err
		}
		return t.momentView(m), nil
	case "list_questions":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		if t.s.questions == nil {
			return nil, domain.ErrUnavailable
		}
		qs, err := t.s.questions.List(ctx, t.actor, t.householdID)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(qs))
		for i, q := range qs {
			out[i] = t.questionView(q)
		}
		return out, nil
	case "ask_question":
		var in struct {
			Text string `json:"text"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.questions == nil {
			return nil, domain.ErrUnavailable
		}
		q, err := t.s.questions.Ask(ctx, t.actor, t.householdID, in.Text)
		if err != nil {
			return nil, err
		}
		return t.questionView(q), nil
	case "answer_question":
		var in struct {
			ID   uuid.UUID `json:"id"`
			Text string    `json:"text"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.questions == nil {
			return nil, domain.ErrUnavailable
		}
		q, err := t.s.questions.Answer(ctx, t.actor, in.ID, in.Text)
		if err != nil {
			return nil, err
		}
		return t.questionView(q), nil
	case "settle_up":
		var in struct {
			To     string  `json:"to"`
			Amount float64 `json:"amount"`
			From   string  `json:"from"`
			Day    string  `json:"day"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		return t.settleUp(ctx, in.From, in.To, in.Amount, in.Day)
	case "complete_gift_idea":
		var in struct {
			ID    uuid.UUID `json:"id"`
			Given *bool     `json:"given"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.gifts == nil {
			return nil, domain.ErrUnavailable
		}
		i, err := t.s.gifts.SetGiven(ctx, t.actor, t.householdID, in.ID, flag(in.Given))
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": i.ID, "for": t.name(i.ForID), "text": i.Text, "given": i.GivenAt != nil}, nil
	case "set_birthday":
		var in struct {
			Month int `json:"month"`
			Day   int `json:"day"`
			Year  int `json:"year"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.gifts == nil {
			return nil, domain.ErrUnavailable
		}
		b := domain.Birthday{Month: time.Month(in.Month), Day: in.Day, Year: in.Year}
		if err := t.s.gifts.SetBirthday(ctx, t.actor, &b); err != nil {
			return nil, err
		}
		return map[string]any{"month": in.Month, "day": in.Day, "year": in.Year}, nil
	case "get_chore_history":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		return t.choreHistory(ctx)
	case "thank_chore":
		var in struct {
			ID   uuid.UUID `json:"id"`
			Line string    `json:"line"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.chores == nil {
			return nil, domain.ErrUnavailable
		}
		d, m, err := t.s.chores.Thank(ctx, t.actor, t.householdID, in.ID, in.Line)
		if err != nil {
			return nil, err
		}
		return map[string]any{"title": d.Title, "by": t.name(d.UserID), "gratitude_written": m != nil}, nil
	case "vote_wish":
		var in struct {
			ID     uuid.UUID `json:"id"`
			Hearts int       `json:"hearts"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.wishes == nil {
			return nil, domain.ErrUnavailable
		}
		w, err := t.s.wishes.Vote(ctx, t.actor, t.householdID, in.ID, in.Hearts)
		if err != nil {
			return nil, err
		}
		return t.wishView(w), nil
	case "complete_wish":
		var in struct {
			ID  uuid.UUID `json:"id"`
			Got *bool     `json:"got"`
		}
		if err := decode(call.Input, &in); err != nil {
			return nil, err
		}
		if t.s.wishes == nil {
			return nil, domain.ErrUnavailable
		}
		w, err := t.s.wishes.Got(ctx, t.actor, t.householdID, in.ID, flag(in.Got))
		if err != nil {
			return nil, err
		}
		return t.wishView(w), nil
	case "get_tonight":
		if err := decode(call.Input, &struct{}{}); err != nil {
			return nil, err
		}
		if t.s.watch == nil {
			return nil, domain.ErrUnavailable
		}
		p, err := t.s.watch.Tonight(ctx, t.actor, t.householdID)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return map[string]any{"picked": false}, nil
		}
		return map[string]any{"picked": true, "name": p.Name, "year": p.Year, "kind": p.Kind, "picked_by": t.name(p.PickedBy)}, nil
	}
	return nil, fmt.Errorf("%w: unknown tool %q", domain.ErrInvalid, call.Name)
}

// flag reads an optional true/false that means true when left out.
func flag(v *bool) bool { return v == nil || *v }

// date reads a day as YYYY-MM-DD, as midnight UTC like the services keep days; "" is no day.
func (t *toolbox) date(field, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s %q is not YYYY-MM-DD", domain.ErrInvalid, field, s)
	}
	return d, nil
}

// today is the household's date as midnight UTC.
func (t *toolbox) today() time.Time {
	y, m, d := t.s.now().In(t.s.loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func formatDay(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.Format(time.DateOnly)
}

func (t *toolbox) updateEntry(ctx context.Context, id uuid.UUID, text, due, starts, ends *string, allDay *bool, repeat *domain.Repeat) (any, error) {
	e, err := t.s.entries.Get(ctx, t.actor, id)
	if err != nil {
		return nil, err
	}
	if e.Sealed {
		return nil, fmt.Errorf("%w: that note is still sealed and can't be changed before it opens", domain.ErrInvalid)
	}
	f := e.EntryFields
	if text != nil {
		f.Text = *text
	}
	for _, c := range []struct {
		in  *string
		out **time.Time
	}{{due, &f.DueAt}, {starts, &f.StartsAt}, {ends, &f.EndsAt}} {
		if c.in == nil {
			continue
		}
		if *c.out, err = t.time(*c.in); err != nil {
			return nil, err
		}
	}
	if allDay != nil {
		f.AllDay = *allDay
	}
	if repeat != nil {
		f.Repeat = *repeat
	}
	e, err = t.s.entries.Update(ctx, t.actor, id, f, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t.view(e), nil
}

func (t *toolbox) mealPlan(ctx context.Context, from string, days int) (any, error) {
	start, err := t.date("from", from)
	if err != nil {
		return nil, err
	}
	if start.IsZero() {
		start = t.today()
	}
	if days <= 0 {
		days = 7
	}
	if days > maxPlanDays {
		days = maxPlanDays
	}
	plan, err := t.s.cook.Plan(ctx, t.actor, t.householdID, start, days)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(plan))
	for i, m := range plan {
		out[i] = t.plannedView(m)
	}
	return out, nil
}

func (t *toolbox) plannedView(m domain.PlannedMeal) map[string]any {
	return map[string]any{"day": m.Day.Format(time.DateOnly), "recipe": m.Recipe.Name, "slug": m.Recipe.Slug, "planned_by": t.name(m.PlannedBy)}
}

func (t *toolbox) ratingView(r domain.MealRating) map[string]any {
	return map[string]any{"dish": r.Name, "slug": r.Slug, "by": t.name(r.UserID), "rating": r.Rating}
}

func bucketView(it domain.BucketItem) map[string]any {
	return map[string]any{"id": it.ID, "kind": it.Kind, "text": it.Text, "done": it.DoneAt != nil}
}

func (t *toolbox) addTrip(ctx context.Context, name, destination, start, end, notes string) (any, error) {
	if t.s.trips == nil {
		return nil, domain.ErrUnavailable
	}
	f := domain.TripFields{Name: name, Destination: destination, Notes: notes}
	for _, c := range []struct {
		field, in string
		out       **time.Time
	}{{"start", start, &f.StartOn}, {"end", end, &f.EndOn}} {
		d, err := t.date(c.field, c.in)
		if err != nil {
			return nil, err
		}
		if !d.IsZero() {
			*c.out = &d
		}
	}
	tr, err := t.s.trips.Create(ctx, t.actor, t.householdID, f)
	if err != nil {
		return nil, err
	}
	return t.tripView(tr), nil
}

// packItem ticks a packing item; the service replaces the whole item, so it is read first.
func (t *toolbox) packItem(ctx context.Context, tripID, itemID uuid.UUID, done bool) (any, error) {
	if t.s.trips == nil {
		return nil, domain.ErrUnavailable
	}
	trips, err := t.s.trips.List(ctx, t.actor, t.householdID)
	if err != nil {
		return nil, err
	}
	for _, tr := range trips {
		if tr.ID != tripID {
			continue
		}
		i := slices.IndexFunc(tr.Packing, func(p domain.PackingItem) bool { return p.ID == itemID })
		if i < 0 {
			break
		}
		tr, err = t.s.trips.UpdatePacking(ctx, t.actor, t.householdID, tripID, itemID, domain.PackingFields{Text: tr.Packing[i].Text, Done: done})
		if err != nil {
			return nil, err
		}
		return t.tripView(tr), nil
	}
	return nil, domain.ErrNotFound
}

func (t *toolbox) tripView(tr domain.Trip) map[string]any {
	options := make([]map[string]any, len(tr.Options))
	for i, o := range tr.Options {
		fans := []string{}
		for _, v := range tr.Votes {
			if v.OptionID == o.ID {
				fans = append(fans, t.name(v.UserID))
			}
		}
		options[i] = map[string]any{"id": o.ID, "title": o.Title, "url": o.URL, "notes": o.Notes, "favoured_by": fans, "chosen": tr.ChosenOptionID != nil && *tr.ChosenOptionID == o.ID}
	}
	packing := make([]map[string]any, len(tr.Packing))
	for i, p := range tr.Packing {
		packing[i] = map[string]any{"id": p.ID, "text": p.Text, "done": p.Done}
	}
	return map[string]any{
		"id": tr.ID, "name": tr.Name, "destination": tr.Destination, "start": formatDay(tr.StartOn), "end": formatDay(tr.EndOn),
		"notes": tr.Notes, "options": options, "packing": packing,
	}
}

func (t *toolbox) moments(ctx context.Context, month string) (any, error) {
	if t.s.momentsSvc == nil {
		return nil, domain.ErrUnavailable
	}
	var m time.Time
	if month = strings.TrimSpace(month); month != "" {
		var err error
		if m, err = time.Parse("2006-01", month); err != nil {
			return nil, fmt.Errorf("%w: month %q is not YYYY-MM", domain.ErrInvalid, month)
		}
	}
	mm, err := t.s.momentsSvc.Month(ctx, t.actor, t.householdID, m)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(mm.Moments))
	for i, x := range mm.Moments {
		out[i] = t.momentView(x)
	}
	return map[string]any{"month": mm.Month.Format("2006-01"), "moments": out, "earlier_month": formatMonth(mm.Earlier)}, nil
}

func (t *toolbox) addMoment(ctx context.Context, kind domain.MomentKind, text string, vis domain.Visibility, day, revealAt string) (any, error) {
	if t.s.momentsSvc == nil {
		return nil, domain.ErrUnavailable
	}
	f := domain.MomentFields{Text: text, Visibility: vis}
	var err error
	if f.Day, err = t.date("day", day); err != nil {
		return nil, err
	}
	if f.RevealAt, err = t.time(revealAt); err != nil {
		return nil, err
	}
	m, err := t.s.momentsSvc.Create(ctx, t.actor, t.householdID, kind, f)
	if err != nil {
		return nil, err
	}
	return t.momentView(m), nil
}

func (t *toolbox) momentView(m domain.Moment) map[string]any {
	v := map[string]any{"id": m.ID, "kind": m.Kind, "by": t.name(m.AuthorID), "day": m.Day.Format(time.DateOnly), "visibility": m.Visibility}
	if m.Sealed {
		v["sealed"] = true
		v["reveal_at"] = t.local(m.RevealAt)
	} else {
		v["text"] = m.Text
	}
	if len(m.Reactions) > 0 {
		reactions := make(map[string]string, len(m.Reactions))
		for _, r := range m.Reactions {
			reactions[t.name(r.UserID)] = r.Emoji
		}
		v["reactions"] = reactions
	}
	return v
}

func (t *toolbox) questionView(q domain.Question) map[string]any {
	answers := make([]map[string]any, len(q.Answers))
	for i, a := range q.Answers {
		v := map[string]any{"by": t.name(a.UserID)}
		if a.Sealed {
			v["sealed"] = true
		} else {
			v["text"] = a.Text
		}
		answers[i] = v
	}
	return map[string]any{"id": q.ID, "text": q.Text, "asked_by": t.name(q.AskedBy), "revealed": q.Revealed(), "answers": answers}
}

func (t *toolbox) settleUp(ctx context.Context, fromName, toName string, amount float64, day string) (any, error) {
	if t.s.budget == nil {
		return nil, domain.ErrUnavailable
	}
	from := t.actor.ID
	if fromName != "" {
		id, err := t.member(fromName)
		if err != nil {
			return nil, err
		}
		from = *id
	}
	to, err := t.member(toName)
	if err != nil {
		return nil, err
	}
	d, err := t.date("day", day)
	if err != nil {
		return nil, err
	}
	e, err := t.s.budget.Settle(ctx, t.actor, t.householdID, from, *to, cents(amount), d)
	if err != nil {
		return nil, err
	}
	return t.expenseView(e), nil
}

func (t *toolbox) choreHistory(ctx context.Context) (any, error) {
	if t.s.chores == nil {
		return nil, domain.ErrUnavailable
	}
	done, err := t.s.chores.History(ctx, t.actor, t.householdID)
	if err != nil {
		return nil, err
	}
	fair, err := t.s.chores.Fairness(ctx, t.actor, t.householdID)
	if err != nil {
		return nil, err
	}
	history := make([]map[string]any, len(done))
	for i, d := range done {
		thanked := make([]string, len(d.ThankedBy))
		for j, id := range d.ThankedBy {
			thanked[j] = t.name(id)
		}
		history[i] = map[string]any{"id": d.ID, "title": d.Title, "points": d.Points, "by": t.name(d.UserID), "day": d.Day.Format(time.DateOnly), "thanked_by": thanked}
	}
	points := func(ta domain.ChoreTally) map[string]int {
		out := make(map[string]int, len(ta.Members))
		for _, m := range ta.Members {
			out[t.name(m.UserID)] = m.Points
		}
		return out
	}
	return map[string]any{"done": history, "points_this_week": points(fair.Week), "points_this_month": points(fair.Month)}, nil
}
