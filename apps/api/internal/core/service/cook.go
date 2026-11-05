package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Cook suggests recipes from Mealie and puts missing ingredients on the household's shopping list.
// Without Mealie (book is nil) cooking is unavailable; without an LLM (chef is nil or down)
// suggestions are random, so the feature degrades instead of breaking.
type Cook struct {
	households port.HouseholdRepository
	book       port.RecipeBook
	chef       port.Chef
	shopping   port.ShoppingService
	meals      port.MealPlanRepository // nil: no meal plan
	taste      port.TasteRepository    // nil: no taste profile
	moments    port.MomentRepository   // nil: no recap
}

func NewCook(households port.HouseholdRepository, book port.RecipeBook, chef port.Chef, shopping port.ShoppingService) *Cook {
	return &Cook{households: households, book: book, chef: chef, shopping: shopping}
}

// WithPlan keeps the household's meal plan in meals.
func (s *Cook) WithPlan(meals port.MealPlanRepository) *Cook {
	s.meals = meals
	return s
}

// WithTaste keeps the household's ratings in taste, and lets the recap read meal memories from
// moments (either may be nil).
func (s *Cook) WithTaste(taste port.TasteRepository, moments port.MomentRepository) *Cook {
	s.taste, s.moments = taste, moments
	return s
}

func (s *Cook) Suggest(ctx context.Context, actor domain.User, householdID uuid.UUID, mood, lang string) (domain.Suggestions, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Suggestions{}, err
	}
	mood = strings.TrimSpace(mood)
	if mood == "" || utf8.RuneCountInString(mood) > domain.MaxMood {
		return domain.Suggestions{}, fmt.Errorf("%w: mood must be 1-%d characters", domain.ErrInvalid, domain.MaxMood)
	}
	all, err := s.book.Recipes(ctx)
	if err != nil {
		return domain.Suggestions{}, err
	}
	if len(all) == 0 {
		return domain.Suggestions{Recipes: []domain.Suggestion{}}, nil
	}

	all, loved := s.tasted(ctx, householdID, all, domain.SuggestionCount)
	if s.chef != nil {
		picks, err := s.chef.Pick(ctx, mood, lang, loved, all, domain.SuggestionCount)
		if err != nil {
			slog.WarnContext(ctx, "chef unavailable, picking at random", "err", err)
		}
		if out := chosen(all, picks, domain.SuggestionCount); len(out) > 0 {
			return domain.Suggestions{Recipes: out, Chef: true}, nil
		}
	}
	return domain.Suggestions{Recipes: random(all, domain.SuggestionCount)}, nil
}

func (s *Cook) Recipe(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string) (domain.Recipe, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Recipe{}, err
	}
	return s.book.Recipe(ctx, slug)
}

func (s *Cook) Image(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string) (domain.RecipeImage, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.RecipeImage{}, err
	}
	return s.book.Image(ctx, slug)
}

func (s *Cook) AddToShoppingList(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string, ingredientIDs []string) (int, error) {
	return s.AddMealsToShoppingList(ctx, actor, householdID, []domain.RecipeIngredients{{Slug: slug, IngredientIDs: ingredientIDs}})
}

func (s *Cook) AddMealsToShoppingList(ctx context.Context, actor domain.User, householdID uuid.UUID, recipes []domain.RecipeIngredients) (int, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return 0, err
	}
	if len(recipes) == 0 || len(recipes) > domain.MaxPlanDays {
		return 0, fmt.Errorf("%w: add from 1-%d recipes", domain.ErrInvalid, domain.MaxPlanDays)
	}
	var items []domain.ShoppingItem
	for _, ri := range recipes {
		r, err := s.book.Recipe(ctx, ri.Slug)
		if err != nil {
			return 0, err
		}
		want := make(map[string]bool, len(ri.IngredientIDs))
		for _, id := range ri.IngredientIDs {
			want[id] = true
		}
		n := len(items)
		for _, in := range r.Ingredients {
			if want[in.ID] {
				items = append(items, domain.ShoppingItem{Text: in.Text, Recipe: r.Name})
			}
		}
		if len(items) == n {
			return 0, fmt.Errorf("%w: none of the ingredients belong to %s", domain.ErrInvalid, ri.Slug)
		}
	}
	added, err := s.shopping.AddItems(ctx, actor, householdID, items)
	return len(added), err
}

func (s *Cook) Find(ctx context.Context, actor domain.User, householdID uuid.UUID, query string, n int) ([]domain.RecipeSummary, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return nil, err
	}
	all, err := s.book.Recipes(ctx)
	if err != nil {
		return nil, err
	}
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		out := []domain.RecipeSummary{}
		for _, r := range random(all, n) {
			out = append(out, r.Recipe)
		}
		return out, nil
	}
	type scored struct {
		r     domain.RecipeSummary
		score int
	}
	var hits []scored
	for _, r := range all {
		name := strings.ToLower(r.Name)
		rest := strings.ToLower(strings.Join(r.Tags, " ") + " " + r.Description)
		score := 0
		for _, w := range words {
			// A word in the name counts more than one in the tags or description.
			if strings.Contains(name, w) {
				score += 2
			} else if strings.Contains(rest, w) {
				score++
			}
		}
		if score > 0 {
			hits = append(hits, scored{r, score})
		}
	}
	slices.SortStableFunc(hits, func(a, b scored) int { return b.score - a.score })
	out := []domain.RecipeSummary{}
	for _, h := range hits[:min(n, len(hits))] {
		out = append(out, h.r)
	}
	return out, nil
}

func (s *Cook) Plan(ctx context.Context, actor domain.User, householdID uuid.UUID, from time.Time, days int) ([]domain.PlannedMeal, error) {
	if err := s.planReady(ctx, actor, householdID, false); err != nil {
		return nil, err
	}
	if days < 1 || days > domain.MaxPlanDays {
		return nil, fmt.Errorf("%w: days must be 1-%d", domain.ErrInvalid, domain.MaxPlanDays)
	}
	from = date(from)
	return s.meals.Meals(ctx, householdID, from, from.AddDate(0, 0, days))
}

func (s *Cook) PlanMeal(ctx context.Context, actor domain.User, householdID uuid.UUID, day time.Time, slug string) (domain.PlannedMeal, error) {
	if err := s.planReady(ctx, actor, householdID, true); err != nil {
		return domain.PlannedMeal{}, err
	}
	r, err := s.book.Recipe(ctx, slug)
	if err != nil {
		return domain.PlannedMeal{}, err
	}
	m := domain.PlannedMeal{HouseholdID: householdID, Day: date(day), Recipe: r.RecipeSummary, PlannedBy: actor.ID}
	if err := s.meals.SaveMeal(ctx, m); err != nil {
		return domain.PlannedMeal{}, err
	}
	return m, nil
}

func (s *Cook) UnplanMeal(ctx context.Context, actor domain.User, householdID uuid.UUID, day time.Time) error {
	if err := s.planReady(ctx, actor, householdID, false); err != nil {
		return err
	}
	return s.meals.DeleteMeal(ctx, householdID, date(day))
}

func (s *Cook) SuggestPlan(ctx context.Context, actor domain.User, householdID uuid.UUID, from time.Time, days int, wish, lang string) (domain.MealIdeas, error) {
	if err := s.planReady(ctx, actor, householdID, true); err != nil {
		return domain.MealIdeas{}, err
	}
	wish = strings.TrimSpace(wish)
	if days < 1 || days > domain.MaxPlanDays || utf8.RuneCountInString(wish) > domain.MaxMood {
		return domain.MealIdeas{}, fmt.Errorf("%w: days must be 1-%d, the wish at most %d characters", domain.ErrInvalid, domain.MaxPlanDays, domain.MaxMood)
	}
	from = date(from)
	to := from.AddDate(0, 0, days)
	past, err := s.meals.Meals(ctx, householdID, from.AddDate(0, 0, -domain.RecentMealDays), to)
	if err != nil {
		return domain.MealIdeas{}, err
	}
	planned := map[time.Time]bool{}
	cooked := map[string]bool{}
	var recent []string
	for _, m := range past {
		if !m.Day.Before(from) {
			planned[m.Day] = true
		}
		if !cooked[m.Recipe.Slug] {
			cooked[m.Recipe.Slug] = true
			recent = append(recent, m.Recipe.Name)
		}
	}
	var open []time.Time
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if !planned[d] {
			open = append(open, d)
		}
	}
	none := domain.MealIdeas{Meals: []domain.MealIdea{}}
	if len(open) == 0 {
		return none, nil
	}
	all, err := s.book.Recipes(ctx)
	if err != nil {
		return domain.MealIdeas{}, err
	}
	all, loved := s.tasted(ctx, householdID, all, len(open))
	// Recipes cooked lately are left out, unless that leaves too few to fill the days.
	fresh := slices.DeleteFunc(slices.Clone(all), func(r domain.RecipeSummary) bool { return cooked[r.Slug] })
	if len(fresh) < len(open) {
		fresh = all
	}
	if len(fresh) == 0 {
		return none, nil
	}

	var picks []domain.Suggestion
	chef := false
	if s.chef != nil {
		ps, err := s.chef.Plan(ctx, wish, lang, s.onHand(ctx, actor, householdID), recent, loved, fresh, len(open))
		if err != nil {
			slog.WarnContext(ctx, "chef unavailable, planning at random", "err", err)
		}
		picks = chosen(fresh, ps, len(open))
		chef = len(picks) > 0
	}
	// Days the chef left open get a random recipe it didn't pick.
	rest := slices.DeleteFunc(slices.Clone(fresh), func(r domain.RecipeSummary) bool {
		return slices.ContainsFunc(picks, func(p domain.Suggestion) bool { return p.Recipe.Slug == r.Slug })
	})
	picks = append(picks, random(rest, len(open)-len(picks))...)

	out := domain.MealIdeas{Meals: make([]domain.MealIdea, 0, len(picks)), Chef: chef}
	for i, p := range picks {
		out.Meals = append(out.Meals, domain.MealIdea{Day: open[i], Recipe: p.Recipe, Reason: p.Reason})
	}
	return out, nil
}

// onHand is what was ticked off the shopping list lately, so it's probably in the kitchen.
func (s *Cook) onHand(ctx context.Context, actor domain.User, householdID uuid.UUID) []string {
	if s.shopping == nil {
		return nil
	}
	items, err := s.shopping.Items(ctx, actor, householdID)
	if err != nil {
		slog.WarnContext(ctx, "reading the shopping list failed", "err", err)
		return nil
	}
	var out []string
	for _, it := range items {
		if it.DoneAt != nil && len(out) < 50 {
			out = append(out, it.Text)
		}
	}
	return out
}

// planReady is ready for the meal plan; reading and changing it works without Mealie.
func (s *Cook) planReady(ctx context.Context, actor domain.User, householdID uuid.UUID, mealie bool) error {
	var err error
	if mealie {
		err = s.ready(ctx, actor, householdID)
	} else {
		_, err = s.households.Role(ctx, householdID, actor.ID)
	}
	if err != nil {
		return err
	}
	if s.meals == nil {
		return fmt.Errorf("%w: no meal plan", domain.ErrUnavailable)
	}
	return nil
}

// date is the calendar date of t, at midnight UTC.
func date(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ready checks membership first, so non-members learn nothing about whether Mealie is set up.
func (s *Cook) ready(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	if s.book == nil {
		return fmt.Errorf("%w: Mealie is not configured", domain.ErrUnavailable)
	}
	return nil
}

// chosen keeps up to n picks that name a real candidate, once each: an LLM can invent slugs.
func chosen(all []domain.RecipeSummary, picks []domain.Pick, n int) []domain.Suggestion {
	bySlug := make(map[string]domain.RecipeSummary, len(all))
	for _, r := range all {
		bySlug[r.Slug] = r
	}
	out := []domain.Suggestion{}
	for _, p := range picks {
		r, ok := bySlug[p.Slug]
		if !ok || len(out) == n {
			continue
		}
		delete(bySlug, p.Slug)
		out = append(out, domain.Suggestion{Recipe: r, Reason: p.Reason})
	}
	return out
}

func random(all []domain.RecipeSummary, n int) []domain.Suggestion {
	out := []domain.Suggestion{}
	for _, i := range rand.Perm(len(all)) { //nolint:gosec // picking dinner, not a secret
		if len(out) == n {
			break
		}
		out = append(out, domain.Suggestion{Recipe: all[i]})
	}
	return out
}

// tasted leaves out the dishes any member said "not again" to, unless that leaves fewer than need,
// and names the dishes loved, those every rater loved first. Without ratings, or when they can't
// be read, it changes nothing.
func (s *Cook) tasted(ctx context.Context, householdID uuid.UUID, all []domain.RecipeSummary, need int) ([]domain.RecipeSummary, []string) {
	if s.taste == nil {
		return all, nil
	}
	ratings, err := s.taste.Ratings(ctx, householdID)
	if err != nil {
		slog.WarnContext(ctx, "reading ratings failed", "err", err)
		return all, nil
	}
	nope := map[string]bool{}
	for _, r := range ratings {
		if r.Rating == domain.RatingNope {
			nope[r.Slug] = true
		}
	}
	kept := slices.DeleteFunc(slices.Clone(all), func(r domain.RecipeSummary) bool { return nope[r.Slug] })
	if len(kept) < need {
		kept = all
	}
	var both, one []string
	for _, d := range dishes(ratings) {
		switch {
		case d.nope || !d.love:
		case d.all && d.raters > 1:
			both = append(both, d.name)
		default:
			one = append(one, d.name)
		}
	}
	return kept, slices.Concat(both, one)[:min(len(both)+len(one), maxLoved)]
}

// maxLoved keeps the prompt short.
const maxLoved = 30

// dish is what the members' ratings of one recipe add up to.
type dish struct {
	slug, name string
	raters     int
	// love is set when anyone loved it; all when everyone who rated it did; nope when anyone
	// said not again.
	love, all, nope bool
}

// dishes groups ratings (newest first) by recipe, in the order of the newest rating.
func dishes(ratings []domain.MealRating) []dish {
	var out []dish
	at := map[string]int{}
	for _, r := range ratings {
		i, ok := at[r.Slug]
		if !ok {
			i = len(out)
			at[r.Slug] = i
			out = append(out, dish{slug: r.Slug, name: r.Name, all: true})
		}
		d := &out[i]
		d.raters++
		d.love = d.love || r.Rating == domain.RatingLove
		d.all = d.all && r.Rating == domain.RatingLove
		d.nope = d.nope || r.Rating == domain.RatingNope
	}
	return out
}

func (s *Cook) Rate(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string, rating domain.Rating) (domain.MealRating, error) {
	if err := s.tasteReady(ctx, actor, householdID, true); err != nil {
		return domain.MealRating{}, err
	}
	if !rating.Valid() {
		return domain.MealRating{}, fmt.Errorf("%w: unknown rating %q", domain.ErrInvalid, rating)
	}
	r, err := s.book.Recipe(ctx, slug)
	if err != nil {
		return domain.MealRating{}, err
	}
	m := domain.MealRating{HouseholdID: householdID, UserID: actor.ID, Slug: r.Slug, Name: r.Name, Rating: rating, RatedAt: time.Now()}
	if err := s.taste.SaveRating(ctx, m); err != nil {
		return domain.MealRating{}, err
	}
	return m, nil
}

func (s *Cook) Unrate(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string) error {
	if err := s.tasteReady(ctx, actor, householdID, false); err != nil {
		return err
	}
	return s.taste.DeleteRating(ctx, householdID, actor.ID, slug)
}

func (s *Cook) Ratings(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]domain.MealRating, error) {
	if err := s.tasteReady(ctx, actor, householdID, false); err != nil {
		return nil, err
	}
	return s.taste.Ratings(ctx, householdID)
}

// tasteReady is ready for ratings; reading and removing them works without Mealie.
func (s *Cook) tasteReady(ctx context.Context, actor domain.User, householdID uuid.UUID, mealie bool) error {
	var err error
	if mealie {
		err = s.ready(ctx, actor, householdID)
	} else {
		_, err = s.households.Role(ctx, householdID, actor.ID)
	}
	if err != nil {
		return err
	}
	if s.taste == nil {
		return fmt.Errorf("%w: no taste profile", domain.ErrUnavailable)
	}
	return nil
}

func (s *Cook) Recap(ctx context.Context, actor domain.User, householdID uuid.UUID, year int) (domain.MealRecap, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.MealRecap{}, err
	}
	if s.moments == nil {
		return domain.MealRecap{}, fmt.Errorf("%w: no meal memories", domain.ErrUnavailable)
	}
	if year == 0 {
		year = time.Now().Year()
	}
	if year < 2000 || year > 9999 {
		return domain.MealRecap{}, fmt.Errorf("%w: year out of range", domain.ErrInvalid)
	}
	from := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	ms, err := s.moments.ListMoments(ctx, householdID, actor.ID, from, from.AddDate(1, 0, 0), domain.MaxRecapMoments)
	if err != nil {
		return domain.MealRecap{}, err
	}
	out := domain.MealRecap{Year: year, Dishes: []domain.CookedDish{}, Favourites: []domain.CookedDish{}, Photos: []domain.Moment{}}
	var cooked []domain.CookedDish
	at := map[string]int{}
	now := time.Now()
	for _, m := range ms { // newest day first
		m = sealMoment(m, actor.ID, now)
		if m.RecipeSlug == "" || m.Kind != domain.KindMoment || m.Sealed {
			continue
		}
		out.Memories++
		i, ok := at[m.RecipeSlug]
		if !ok {
			i = len(cooked)
			at[m.RecipeSlug] = i
			cooked = append(cooked, domain.CookedDish{Slug: m.RecipeSlug, Name: m.RecipeName, Last: m.Day})
		}
		cooked[i].Times++
		if m.PhotoAt != nil && len(out.Photos) < domain.MaxRecapPhotos {
			out.Photos = append(out.Photos, m)
		}
	}
	// Favourites come from every rating, not only this year's dishes.
	if s.taste != nil {
		ratings, err := s.taste.Ratings(ctx, householdID)
		if err != nil {
			return domain.MealRecap{}, err
		}
		for _, d := range dishes(ratings) {
			if d.all && d.raters > 1 {
				if i, ok := at[d.slug]; ok {
					out.Favourites = append(out.Favourites, cooked[i])
				}
			}
		}
	}
	slices.SortStableFunc(cooked, func(a, b domain.CookedDish) int { return b.Times - a.Times })
	out.Dishes = cooked[:min(len(cooked), domain.MaxRecapDishes)]
	return out, nil
}
