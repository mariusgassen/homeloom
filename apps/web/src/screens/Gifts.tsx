import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { api, type GiftIdea, type GiftPerson, type Occasion } from "../api/client";
import { birthdayValue, parseBirthday, turns, useGifts } from "../gifts";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { Swipe } from "../ui/Swipe";
import { Screen } from "./Screen";

// How far off a birthday is, in words.
export function useInDays() {
  const { t } = useTranslation();
  return (days: number) => (days === 0 ? t("gifts.today") : days === 1 ? t("gifts.tomorrow") : t("gifts.inDays", { count: days }));
}

// The gift ideas page, opened from Today or Settings: for each of the others, their birthday and
// the ideas only you can see, and your own birthday so they can do the same for you.
export function Gifts() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("page without a household");
  const household = session.household;
  const { gifts, failed, reload } = useGifts(household.id);
  const [error, setError] = useState(false);
  const inDays = useInDays();
  const path = { params: { path: { householdId: household.id } } };

  async function run(call: Promise<{ error?: unknown }>) {
    const res = await call.catch(() => ({ error: true }));
    setError(!!res.error);
    reload();
    return !res.error;
  }

  const idea = (id: string) => ({ params: { path: { householdId: household.id, ideaId: id } } });
  const given = (i: GiftIdea) => run(api.PATCH("/v1/households/{householdId}/gifts/ideas/{ideaId}", { ...idea(i.id), body: { given: !i.given } }));
  const remove = (i: GiftIdea) => run(api.DELETE("/v1/households/{householdId}/gifts/ideas/{ideaId}", idea(i.id)));
  const add = (forId: string, text: string, occasionId?: string) =>
    run(api.POST("/v1/households/{householdId}/gifts/ideas", { ...path, body: { forId, text, occasionId } }));
  const addDate = (name: string, value: string) => {
    const date = parseBirthday(value);
    return date ? run(api.POST("/v1/households/{householdId}/gifts/occasions", { ...path, body: { name, month: date.month, day: date.day } })) : Promise.resolve(false);
  };
  const removeDate = (o: Occasion) =>
    run(api.DELETE("/v1/households/{householdId}/gifts/occasions/{occasionId}", { params: { path: { householdId: household.id, occasionId: o.id } } }));
  const setBirthday = (value: string) => {
    const body = parseBirthday(value);
    return body ? run(api.PUT("/v1/me/birthday", { body })) : Promise.resolve(false);
  };

  return (
    <Screen
      title={t("gifts.title")}
      back={
        <Button variant="plain" size="sm" className="back" onClick={() => navigate("/")}>
          <span className="back-icon">
            <Icon name="chevron" size="sm" />
          </span>
          {t("tabs.today")}
        </Button>
      }
    >
      <p className="caption muted gifts-private">
        <Icon name="lock" size="sm" /> {t("gifts.private")}
      </p>
      {(failed || error) && (
        <p className="callout error" role="alert">
          {t("gifts.failed")}
        </p>
      )}
      {gifts?.people.length === 0 && <Empty title={t("gifts.emptyTitle")}>{t("gifts.emptyBody")}</Empty>}
      {gifts?.people.map((p) => (
        <Person key={p.user.id} person={p} ideas={gifts.ideas.filter((i) => i.forId === p.user.id)} occasions={gifts.occasions} when={inDays} onAdd={add} onGiven={given} onDelete={remove} />
      ))}
      {gifts && <Dates occasions={gifts.occasions} when={inDays} onAdd={addDate} onDelete={removeDate} />}
      {gifts && (
        <section className="group">
          <h2 className="label group-title">{t("gifts.mine")}</h2>
          <div className="hl-list gifts-mine">
            <input
              className="input"
              type="date" min="1900-01-01" max={new Date().toISOString().slice(0, 10)}
              aria-label={t("gifts.mine")}
              value={gifts.myBirthday ? birthdayValue(gifts.myBirthday) : ""}
              onChange={(e) => void setBirthday(e.target.value)}
            />
            {gifts.myBirthday && (
              <Button variant="plain" size="sm" onClick={() => void run(api.DELETE("/v1/me/birthday"))}>
                {t("gifts.remove")}
              </Button>
            )}
          </div>
          <p className="caption muted">{t("gifts.mineBody")}</p>
        </section>
      )}
    </Screen>
  );
}

function Dates({ occasions, when, onAdd, onDelete }: {
  occasions: Occasion[];
  when: (days: number) => string;
  onAdd: (name: string, date: string) => Promise<boolean>;
  onDelete: (o: Occasion) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [date, setDate] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (name.trim() && date && (await onAdd(name, date))) {
      setName("");
      setDate("");
    }
  }
  return (
    <section className="group">
      <h2 className="label group-title">{t("gifts.dates")}</h2>
      {occasions.length > 0 && (
        <div className="hl-list">
          {occasions.map((o) => (
            <Swipe key={o.id} end={{ label: t("gifts.dateDelete"), icon: "trash", tone: "danger", onAction: () => void onDelete(o) }}>
              <ListRow title={o.name} subtitle={when(o.days)} />
            </Swipe>
          ))}
        </div>
      )}
      <form className="shopping-add" onSubmit={submit}>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("gifts.datePlaceholder")} aria-label={t("gifts.datePlaceholder")} maxLength={60} enterKeyHint="done" />
        <input className="input" type="date" value={date} onChange={(e) => setDate(e.target.value)} aria-label={t("gifts.dateWhen")} />
        <Button type="submit" icon="plus" disabled={!name.trim() || !date} aria-label={t("gifts.dateAdd")} />
      </form>
    </section>
  );
}

function Person({ person, ideas, occasions, when, onAdd, onGiven, onDelete }: {
  person: GiftPerson;
  ideas: GiftIdea[];
  occasions: Occasion[];
  when: (days: number) => string;
  onAdd: (forId: string, text: string, occasionId?: string) => Promise<boolean>;
  onGiven: (i: GiftIdea) => Promise<boolean>;
  onDelete: (i: GiftIdea) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const [occasionId, setOccasionId] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (text.trim() && (await onAdd(person.user.id, text, occasionId || undefined))) setText("");
  }
  return (
    <section className="group">
      <h2 className="label group-title">{person.user.name}</h2>
      <p className="caption muted">{person.days === undefined ? t("gifts.birthdayNone") : `${t("gifts.birthday", { name: person.user.name })} · ${when(person.days)}${turns(person) ? ` · ${t("gifts.turns", { count: turns(person) })}` : ""}`}</p>
      {ideas.length > 0 && (
        <div className="hl-list">
          {ideas.map((i) => (
            <Swipe
              key={i.id}
              start={{ label: t(i.given ? "gifts.reopen" : "gifts.given"), icon: "check", tone: "done", onAction: () => void onGiven(i) }}
              end={{ label: t("gifts.delete"), icon: "trash", tone: "danger", onAction: () => void onDelete(i) }}
            >
              <div className="entry-row">
                <button type="button" className={i.given ? "tick tick-done" : "tick"} role="checkbox" aria-checked={i.given} aria-label={i.text} onClick={() => void onGiven(i)}>
                  {i.given && <Icon name="check" size="sm" />}
                </button>
                <ListRow title={<span className={i.given ? "have" : undefined}>{i.text}</span>} subtitle={occasions.find((o) => o.id === i.occasionId)?.name} />
              </div>
            </Swipe>
          ))}
        </div>
      )}
      <form className="shopping-add" onSubmit={submit}>
        <input className="input" value={text} onChange={(e) => setText(e.target.value)} placeholder={t("gifts.placeholder", { name: person.user.name })} aria-label={t("gifts.placeholder", { name: person.user.name })} maxLength={200} enterKeyHint="done" />
        <Button type="submit" icon="plus" disabled={!text.trim()} aria-label={t("gifts.add")} />
      </form>
      {occasions.length > 0 && (
        <select className="input" value={occasionId} onChange={(e) => setOccasionId(e.target.value)} aria-label={t("gifts.forOccasion")}>
          <option value="">{t("gifts.anyTime")}</option>
          {occasions.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name}
            </option>
          ))}
        </select>
      )}
    </section>
  );
}
