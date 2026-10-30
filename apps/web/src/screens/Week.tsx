import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router";
import { api, type MealIdea, type Recipe } from "../api/client";
import { nextDays, useDayLabel, useMealPlan } from "../meals";
import { useRefresh } from "../refresh";
import { Button } from "../ui/Button";
import { cx, Icon } from "../ui/Icon";
import { RecipePhoto, useHouseholdId } from "./Cook";
import { Screen } from "./Screen";

const weekLength = 7;

function Back({ to, label }: { to: string; label: string }) {
  const navigate = useNavigate();
  return (
    <Button variant="plain" size="sm" className="back" onClick={() => navigate(to)}>
      <span className="back-icon">
        <Icon name="chevron" size="sm" />
      </span>
      {label}
    </Button>
  );
}

// The coming seven dinners. Empty days can be filled with ideas, which are only planned once
// accepted; planned days lead to their recipe.
export function Week() {
  const { t } = useTranslation();
  const householdId = useHouseholdId();
  const days = nextDays(weekLength);
  const { meals, plan, unplan } = useMealPlan(householdId, days);
  const label = useDayLabel();
  const [wish, setWish] = useState("");
  const [ideas, setIdeas] = useState<MealIdea[]>([]);
  const [random, setRandom] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const open = days.filter((d) => meals && !meals[d]);
  const ideaFor = (day: string) => ideas.find((i) => i.day === day);
  const planned = days.filter((d) => meals?.[d]);

  async function suggest(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/meal-plan/suggestions", {
        params: { path: { householdId } },
        body: { from: days[0] ?? "", days: weekLength, ...(wish.trim() && { wish: wish.trim() }) },
      })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) {
      setIdeas(res.data.meals);
      setRandom(!res.data.chef && res.data.meals.length > 0);
      if (res.data.meals.length === 0) setError(t("week.noIdeas"));
    } else {
      setError(res?.error?.message ?? t("cook.failed"));
    }
  }

  async function accept(idea: MealIdea) {
    setIdeas((all) => all.filter((i) => i !== idea));
    const failed = await plan(idea.day, idea.recipe.slug);
    if (failed !== null) setError(failed || t("cook.failed"));
  }

  return (
    <Screen title={t("week.title")} back={<Back to="/cook" label={t("cook.title")} />}>
      <ol className="week" aria-label={t("week.title")}>
        {days.map((day) => {
          const meal = meals?.[day];
          const idea = !meal ? ideaFor(day) : undefined;
          return (
            <li key={day} className="week-day">
              <span className="label week-day-label">{label(day)}</span>
              {meal ? (
                <div className="week-meal">
                  <Link to={`/cook/${meal.recipe.slug}`} className="week-meal-main">
                    <RecipePhoto src={meal.recipe.imageUrl} className="week-thumb" />
                    <span className="week-meal-text">
                      <span className="headline">{meal.recipe.name}</span>
                      {meal.recipe.totalTime && <span className="caption muted">{meal.recipe.totalTime}</span>}
                    </span>
                  </Link>
                  <button type="button" className="week-action" aria-label={t("week.remove", { name: meal.recipe.name })} onClick={() => unplan(day)}>
                    <Icon name="close" size="sm" />
                  </button>
                </div>
              ) : idea ? (
                <div className="week-meal week-idea">
                  <Link to={`/cook/${idea.recipe.slug}`} className="week-meal-main">
                    <RecipePhoto src={idea.recipe.imageUrl} className="week-thumb" />
                    <span className="week-meal-text">
                      <span className="headline">{idea.recipe.name}</span>
                      {idea.reason && <span className="caption muted">{idea.reason}</span>}
                    </span>
                  </Link>
                  <button type="button" className="week-action week-accept" aria-label={t("week.accept", { name: idea.recipe.name })} onClick={() => accept(idea)}>
                    <Icon name="check" size="sm" />
                  </button>
                  <button type="button" className="week-action" aria-label={t("week.dismiss", { name: idea.recipe.name })} onClick={() => setIdeas((all) => all.filter((i) => i !== idea))}>
                    <Icon name="close" size="sm" />
                  </button>
                </div>
              ) : (
                <span className="body muted week-empty">{meals ? t("week.nothing") : "…"}</span>
              )}
            </li>
          );
        })}
      </ol>
      {random && <p className="caption muted">{t("week.random")}</p>}
      {ideas.length > 1 && (
        <Button variant="secondary" icon="check" block onClick={() => ideas.forEach(accept)}>
          {t("week.acceptAll")}
        </Button>
      )}
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}

      {open.length > 0 && (
        <form className="cook-form" onSubmit={suggest}>
          <label className="cook-group">
            <span className="label cook-label">{t("cook.useUp")}</span>
            <input className="input" value={wish} onChange={(e) => setWish(e.target.value)} placeholder={t("cook.useUpPlaceholder")} maxLength={200} />
          </label>
          <Button type="submit" icon="sparkle" block disabled={busy}>
            {t(busy ? "cook.thinking" : "week.fill", { count: open.length })}
          </Button>
        </form>
      )}

      {planned.length > 0 && (
        <Link to="/cook/week/shopping" className="hl-btn hl-btn-secondary hl-btn-block week-shop">
          {t("week.shopping")}
        </Link>
      )}
    </Screen>
  );
}

// Every ingredient of the week's dinners, recipe by recipe. Tick what you have; the rest goes
// to the shopping list in one go, sorted into aisles there.
export function WeekShopping() {
  const { t } = useTranslation();
  const householdId = useHouseholdId();
  const days = nextDays(weekLength);
  const { meals } = useMealPlan(householdId, days);
  const [recipes, setRecipes] = useState<Recipe[]>();
  const [have, setHave] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [added, setAdded] = useState<string | null>(null);
  const { track } = useRefresh();

  // Each recipe once, in the order they're cooked.
  const slugs = [...new Set(days.map((d) => meals?.[d]?.recipe.slug).filter((s): s is string => Boolean(s)))];
  const key = slugs.join(",");

  useEffect(() => {
    if (!meals) return;
    let current = true;
    const request = Promise.all(
      key
        .split(",")
        .filter(Boolean)
        .map((slug) =>
          api
            .GET("/v1/households/{householdId}/recipes/{slug}", { params: { path: { householdId, slug } } })
            .catch(() => undefined)
            .then((res) => res?.data),
        ),
    ).then((all) => {
      if (!current) return;
      const found = all.filter((r): r is Recipe => Boolean(r));
      setRecipes(found);
      if (found.length < all.length) setError(t("cook.failed"));
    });
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, key, meals, t, track]);

  // Ingredient ids are only unique within a recipe.
  const id = (r: Recipe, ingredient: string) => `${r.slug}/${ingredient}`;
  const missing = (recipes ?? [])
    .map((r) => ({ recipe: r.slug, ingredientIds: r.ingredients.filter((i) => !have.includes(id(r, i.id))).map((i) => i.id) }))
    .filter((r) => r.ingredientIds.length > 0);
  const count = missing.reduce((n, r) => n + r.ingredientIds.length, 0);

  async function addMissing() {
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/meal-plan/shopping-list", { params: { path: { householdId } }, body: { recipes: missing } })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) setAdded(t("cook.added", { count: res.data.added, list: res.data.list }));
    else setError(res?.error?.message ?? t("cook.failed"));
  }

  const toggle = (key: string) => setHave((h) => (h.includes(key) ? h.filter((x) => x !== key) : [...h, key]));

  return (
    <Screen title={t("week.shopping")} back={<Back to="/cook/week" label={t("week.title")} />}>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {recipes && recipes.length === 0 && !error && <p className="body muted">{t("week.nothingPlanned")}</p>}
      {recipes && recipes.length > 0 && <p className="caption muted">{t("cook.haveHint")}</p>}
      {recipes?.map((r) => (
        <section key={r.slug} className="group" aria-label={r.name}>
          <h2 className="label group-title">{r.name}</h2>
          <div className="hl-list">
            {r.ingredients.map((i) => (
              <label key={i.id} className="hl-listrow check-row">
                <input type="checkbox" checked={have.includes(id(r, i.id))} onChange={() => toggle(id(r, i.id))} />
                <span className="hl-listrow-main">
                  <span className={cx("hl-listrow-title", have.includes(id(r, i.id)) && "have")}>{i.text}</span>
                </span>
              </label>
            ))}
          </div>
        </section>
      ))}
      {recipes && recipes.length > 0 &&
        (added ? (
          <p className="callout success" role="status">
            {added} <Link to="/lists?show=shopping">{t("shopping.open")}</Link>
          </p>
        ) : (
          <Button block disabled={busy || count === 0} onClick={addMissing}>
            {t("cook.addMissing", { count })}
          </Button>
        ))}
    </Screen>
  );
}
