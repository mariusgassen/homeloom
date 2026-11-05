import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { api, type DateIdea, type DateNight as Ideas } from "../api/client";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";
import { Segmented } from "../ui/Form";
import { Screen } from "./Screen";

type Budget = "free" | "easy" | "treat";

const placeKey = "homeloom.datePlace";
const evening = 2 * 60 * 60 * 1000;

// The calendar date of d on this device, as YYYY-MM-DD.
const isoDay = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

function rememberedPlace() {
  try {
    return localStorage.getItem(placeKey) ?? "";
  } catch {
    return "";
  }
}

// A few ideas for an evening together, from the weather where you are, what's on, a budget and a
// mood. A chosen idea goes into the shared calendar.
export function DateNight() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { session } = useSession();
  const householdId = session.state === "signedIn" ? session.household?.id : undefined;
  const today = new Date();
  const last = new Date(today.getTime() + 14 * 24 * 60 * 60 * 1000);
  const [place, setPlace] = useState(rememberedPlace);
  const [day, setDay] = useState(isoDay(today));
  const [budget, setBudget] = useState<Budget>("easy");
  const [mood, setMood] = useState("");
  const [result, setResult] = useState<Ideas | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function ask(e: FormEvent) {
    e.preventDefault();
    if (!householdId) return;
    const where = place.trim();
    try {
      localStorage.setItem(placeKey, where);
    } catch {
      // Private mode: it just isn't remembered.
    }
    setBusy(true);
    setError(null);
    const res = await api
      .POST("/v1/households/{householdId}/date-night/ideas", {
        params: { path: { householdId } },
        body: { place: where || undefined, day, budget, mood: mood.trim() || undefined },
      })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) setResult(res.data);
    else setError(res?.error?.message ?? t("dateNight.failed"));
  }

  const back = (
    <Button variant="plain" size="sm" className="back" onClick={() => navigate("/")}>
      <span className="back-icon">
        <Icon name="chevron" size="sm" />
      </span>
      {t("tabs.today")}
    </Button>
  );
  return (
    <Screen title={t("dateNight.title")} back={back}>
      <form className="hl-card date-form" onSubmit={ask}>
        <label className="field">
          <span className="label">{t("dateNight.place")}</span>
          <input className="input" value={place} onChange={(e) => setPlace(e.target.value)} placeholder={t("dateNight.placeHint")} maxLength={100} autoComplete="address-level2" />
        </label>
        <label className="field">
          <span className="label">{t("dateNight.day")}</span>
          <input className="input" type="date" value={day} min={isoDay(today)} max={isoDay(last)} onChange={(e) => e.target.value && setDay(e.target.value)} />
        </label>
        <Segmented
          label={t("dateNight.budget.label")}
          value={budget}
          onChange={setBudget}
          options={[
            { value: "free", label: t("dateNight.budget.free") },
            { value: "easy", label: t("dateNight.budget.easy") },
            { value: "treat", label: t("dateNight.budget.treat") },
          ]}
        />
        <label className="field">
          <span className="label">{t("dateNight.mood")}</span>
          <input className="input" value={mood} onChange={(e) => setMood(e.target.value)} placeholder={t("dateNight.moodHint")} maxLength={200} enterKeyHint="go" />
        </label>
        <div className="hl-card-footer">
          <Button type="submit" size="sm" icon="sparkle" disabled={busy}>
            {t(busy ? "dateNight.thinking" : result ? "dateNight.more" : "dateNight.ask")}
          </Button>
        </div>
        {error && (
          <p className="callout error" role="alert">
            {error}
          </p>
        )}
      </form>
      {result?.forecast && (
        <p className="callout muted date-weather">
          {t(`dateNight.sky.${result.forecast.sky}`)} · {Math.round(result.forecast.lowC)}–{Math.round(result.forecast.highC)}°
          {result.forecast.rainPercent >= 20 && ` · ${t("dateNight.rain", { percent: result.forecast.rainPercent })}`}
        </p>
      )}
      {result?.ideas.map((idea) => (
        <Idea key={idea.title + idea.startsAt} idea={idea} householdId={householdId} lang={i18n.language} />
      ))}
    </Screen>
  );
}

function Idea({ idea, householdId, lang }: { idea: DateIdea; householdId?: string; lang: string }) {
  const { t } = useTranslation();
  const [state, setState] = useState<"idle" | "saving" | "saved" | "failed">("idle");
  const starts = new Date(idea.startsAt);
  const when = new Intl.DateTimeFormat(lang, { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }).format(starts);
  const facts = [when, idea.where, idea.cost, t(idea.indoor ? "dateNight.indoor" : "dateNight.outdoor")].filter(Boolean);

  async function save() {
    if (!householdId) return;
    setState("saving");
    const res = await api
      .POST("/v1/households/{householdId}/entries", {
        params: { path: { householdId } },
        body: {
          kind: "event",
          text: idea.title,
          visibility: "shared",
          repeat: "never",
          allDay: false,
          startsAt: starts.toISOString(),
          endsAt: new Date(starts.getTime() + evening).toISOString(),
        },
      })
      .catch(() => undefined);
    setState(res?.data ? "saved" : "failed");
  }

  return (
    <section className="hl-card date-idea">
      <span className="headline">{idea.title}</span>
      <span className="caption muted">{facts.join(" · ")}</span>
      <span className="body">{idea.reason}</span>
      <div className="hl-card-footer">
        {state === "saved" ? (
          <span className="caption muted" role="status">
            <Icon name="check" size="sm" /> {t("dateNight.saved")}
          </span>
        ) : (
          <Button size="sm" variant="secondary" icon="calendar" disabled={state === "saving"} onClick={save}>
            {t("dateNight.save")}
          </Button>
        )}
        {state === "failed" && (
          <span className="caption error" role="alert">
            {t("dateNight.failed")}
          </span>
        )}
      </div>
    </section>
  );
}
