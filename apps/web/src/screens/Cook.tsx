import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate, useParams } from "react-router";
import { api, type Recipe, type Suggestions } from "../api/client";
import { useRefresh } from "../refresh";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Empty } from "../ui/Empty";
import { cx, Icon } from "../ui/Icon";
import { Screen } from "./Screen";

const moods = ["cozy", "light", "spicy", "comfort", "new"] as const;
const times = ["quick", "medium", "slow"] as const;

// The last suggestions survive a trip to a recipe and back.
const lastKey = "homeloom.cook";
function loadLast(): Suggestions | null {
  try {
    const raw = sessionStorage.getItem(lastKey);
    return raw ? (JSON.parse(raw) as Suggestions) : null;
  } catch {
    return null;
  }
}
function saveLast(s: Suggestions) {
  try {
    sessionStorage.setItem(lastKey, JSON.stringify(s));
  } catch {
    // Without storage the suggestions just don't come back.
  }
}

function useHouseholdId() {
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("cook without a household");
  return session.household.id;
}

export function Cook() {
  const { t } = useTranslation();
  const householdId = useHouseholdId();
  const [picked, setPicked] = useState<string[]>([]);
  const [time, setTime] = useState<string | null>(null);
  const [useUp, setUseUp] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Suggestions | null>(loadLast);

  // The wish goes to Claude as one sentence in the user's language.
  const wish = [
    picked.map((m) => t(`cook.mood.${m}`)).join(", "),
    time && `${t("cook.timeLabel")}: ${t(`cook.time.${time}`)}`,
    useUp.trim() && `${t("cook.useUp")}: ${useUp.trim()}`,
  ]
    .filter(Boolean)
    .join("; ");

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/cook/suggestions", { params: { path: { householdId } }, body: { mood: wish } })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) {
      setResult(res.data);
      saveLast(res.data);
    } else {
      setError(res?.error?.message ?? t("cook.failed"));
    }
  }

  const toggle = (m: string) => setPicked((p) => (p.includes(m) ? p.filter((x) => x !== m) : [...p, m]));

  return (
    <Screen title={t("tabs.cook")} refreshable={false}>
      <form className="cook-form" onSubmit={submit}>
        <h2 className="title">{t("cook.question")}</h2>
        <div className="cook-group" role="group" aria-label={t("cook.moodLabel")}>
          <span className="label cook-label">{t("cook.moodLabel")}</span>
          <div className="cook-chips">
            {moods.map((m) => (
              <button key={m} type="button" className={cx("hl-chip", picked.includes(m) && "hl-chip-shared")} aria-pressed={picked.includes(m)} onClick={() => toggle(m)}>
                {t(`cook.mood.${m}`)}
              </button>
            ))}
          </div>
        </div>
        <div className="cook-group" role="group" aria-label={t("cook.timeLabel")}>
          <span className="label cook-label">{t("cook.timeLabel")}</span>
          <div className="cook-times">
            {times.map((x) => (
              <button key={x} type="button" className={cx("hl-chip", time === x && "hl-chip-shared")} aria-pressed={time === x} onClick={() => setTime(time === x ? null : x)}>
                {t(`cook.time.${x}`)}
              </button>
            ))}
          </div>
        </div>
        <label className="cook-group">
          <span className="label cook-label">{t("cook.useUp")}</span>
          <input className="input" value={useUp} onChange={(e) => setUseUp(e.target.value)} placeholder={t("cook.useUpPlaceholder")} maxLength={100} />
        </label>
        {error && (
          <p className="callout error" role="alert">
            {error}
          </p>
        )}
        <Button type="submit" icon="sparkle" block disabled={busy || !wish}>
          {t(busy ? "cook.thinking" : "cook.suggest")}
        </Button>
      </form>

      {result && (
        <section className="group" aria-label={t("cook.suggestions")}>
          <h2 className="label group-title">{t("cook.suggestions")}</h2>
          {!result.chef && result.suggestions.length > 0 && <p className="caption muted">{t("cook.random")}</p>}
          {result.suggestions.length === 0 && (
            <Empty title={t("cook.emptyTitle")}>{t("cook.emptyBody")}</Empty>
          )}
          {result.suggestions.map(({ recipe, reason }) => (
            <Link key={recipe.slug} to={`/cook/${recipe.slug}`} className="card-link">
              <Card
                eyebrow={<span>{[recipe.totalTime, ...recipe.tags.slice(0, 2)].filter(Boolean).join(" · ")}</span>}
                title={recipe.name}
              >
                {reason ?? recipe.description}
              </Card>
            </Link>
          ))}
        </section>
      )}
    </Screen>
  );
}

export function RecipeScreen() {
  const { t } = useTranslation();
  const householdId = useHouseholdId();
  const navigate = useNavigate();
  const { slug = "" } = useParams();
  const [recipe, setRecipe] = useState<Recipe | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [have, setHave] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [added, setAdded] = useState<string | null>(null);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/recipes/{slug}", { params: { path: { householdId, slug } } })
      .catch(() => undefined)
      .then((res) => {
        if (!current) return;
        if (res?.data) setRecipe(res.data);
        setError(res?.data ? null : (res?.error?.message ?? t("cook.failed")));
      });
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, slug, t, refreshed, track]);

  const missing = recipe?.ingredients.filter((i) => !have.includes(i.id)) ?? [];

  async function addMissing() {
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/shopping-list", {
        params: { path: { householdId } },
        body: { recipe: slug, ingredientIds: missing.map((i) => i.id) },
      })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) setAdded(t("cook.added", { count: res.data.added, list: res.data.list }));
    else setError(res?.error?.message ?? t("cook.failed"));
  }

  return (
    <Screen
      title={recipe?.name ?? ""}
      back={
        <Button variant="plain" size="sm" className="back" onClick={() => navigate("/cook")}>
          <span className="back-icon">
            <Icon name="chevron" size="sm" />
          </span>
          {t("tabs.cook")}
        </Button>
      }
    >
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {recipe && (
        <>
          {(recipe.description ?? recipe.totalTime ?? recipe.servings) && (
            <p className="body muted recipe-intro">
              {[recipe.totalTime, recipe.servings && t("cook.servings", { servings: recipe.servings })].filter(Boolean).join(" · ")}
              {recipe.description && <span className="recipe-description">{recipe.description}</span>}
            </p>
          )}
          {recipe.ingredients.length > 0 && (
            <section className="group">
              <h2 className="label group-title">{t("cook.ingredients")}</h2>
              <p className="caption muted">{t("cook.haveHint")}</p>
              <div className="hl-list">
                {recipe.ingredients.map((i) => (
                  <label key={i.id} className="hl-listrow check-row">
                    <input
                      type="checkbox"
                      checked={have.includes(i.id)}
                      onChange={() => setHave((h) => (h.includes(i.id) ? h.filter((x) => x !== i.id) : [...h, i.id]))}
                    />
                    <span className="hl-listrow-main">
                      <span className={cx("hl-listrow-title", have.includes(i.id) && "have")}>{i.text}</span>
                    </span>
                  </label>
                ))}
              </div>
              {added ? (
                <p className="callout success" role="status">
                  {added} <Link to="/lists?show=shopping">{t("shopping.open")}</Link>
                </p>
              ) : (
                <Button block disabled={busy || missing.length === 0} onClick={addMissing}>
                  {t("cook.addMissing", { count: missing.length })}
                </Button>
              )}
            </section>
          )}
          {recipe.steps.length > 0 && (
            <section className="group">
              <h2 className="label group-title">{t("cook.steps")}</h2>
              <ol className="steps">
                {recipe.steps.map((s, n) => (
                  <li key={n} className="body">
                    {s}
                  </li>
                ))}
              </ol>
            </section>
          )}
          {recipe.mealieUrl && (
            <a className="body mealie-link" href={recipe.mealieUrl} target="_blank" rel="noreferrer">
              {t("cook.openRecipe")}
            </a>
          )}
        </>
      )}
    </Screen>
  );
}
