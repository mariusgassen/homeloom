import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router";
import { api, type HouseholdDetail, type PackingItem, type Trip, type TripOption, type User } from "../api/client";
import { cover, leading, packed, useTrips, useTripDates } from "../trips";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { cx, Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { Swipe } from "../ui/Swipe";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";
import { Screen } from "./Screen";
import { OptionSheet, TripSheet } from "./TripSheets";

function useHousehold() {
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("page without a household");
  return { me: session.user, household: session.household };
}

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

// The trips page, opened from Today: every trip the two of you are planning.
export function Trips() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { household } = useHousehold();
  const { trips, failed, reload } = useTrips(household.id);
  const dates = useTripDates();
  const [sheet, setSheet] = useState(false);
  return (
    <Screen title={t("trips.title")} back={<Back to="/" label={t("tabs.today")} />} add={{ label: t("trips.new"), onClick: () => setSheet(true) }}>
      {failed && (
        <p className="callout error" role="alert">
          {t("trips.failed")}
        </p>
      )}
      {trips?.length === 0 && (
        <Empty
          title={t("trips.emptyTitle")}
          action={
            <Button size="sm" onClick={() => setSheet(true)}>
              {t("trips.new")}
            </Button>
          }
        >
          {t("trips.emptyBody")}
        </Empty>
      )}
      {trips && trips.length > 0 && (
        <div className="hl-list">
          {trips.map((trip) => {
            const photo = cover(trip);
            return (
              <ListRow
                key={trip.id}
                leading={photo ? <img className="trip-cover" src={photo} alt="" /> : <span className="trip-cover trip-cover-empty"><Icon name="suitcase" /></span>}
                title={trip.name}
                subtitle={[trip.destination, dates(trip), trip.options.length > 0 && t("trips.places", { count: trip.options.length })].filter(Boolean).join(" · ")}
                onClick={() => navigate(`/trips/${trip.id}`)}
              />
            );
          })}
        </div>
      )}
      {sheet && (
        <TripSheet
          householdId={household.id}
          onClose={() => setSheet(false)}
          onSaved={(trip) => {
            setSheet(false);
            navigate(`/trips/${trip.id}`);
            reload();
          }}
        />
      )}
    </Screen>
  );
}

// One trip: the places the two of you are weighing up, with photos and who favours which, and
// what to pack.
export function TripPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { id } = useParams();
  const { me, household } = useHousehold();
  const { trips, apply, reload } = useTrips(household.id);
  const dates = useTripDates();
  const [sheet, setSheet] = useState<"trip" | TripOption | "option" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const trip = trips?.find((x) => x.id === id);
  const back = <Back to="/trips" label={t("trips.title")} />;
  if (!trip) {
    return (
      <Screen title={t("trips.title")} back={back}>
        {trips && (
          <Empty title={t("trips.missingTitle")}>
            <Button size="sm" onClick={() => navigate("/trips")}>
              {t("trips.title")}
            </Button>
          </Empty>
        )}
      </Screen>
    );
  }

  const path = { householdId: household.id, tripId: trip.id };
  // Runs a change that answers with the trip, and shows it.
  async function change(request: Promise<{ data?: Trip; error?: { message: string } }>) {
    const res = await request.catch(() => undefined);
    setError(res?.data ? null : (res?.error?.message ?? t("trips.failed")));
    if (res?.data) apply(res.data);
  }
  const vote = (o: TripOption) =>
    o.votes.includes(me.id)
      ? change(api.DELETE("/v1/households/{householdId}/trips/{tripId}/vote", { params: { path } }))
      : change(api.PUT("/v1/households/{householdId}/trips/{tripId}/options/{optionId}/vote", { params: { path: { ...path, optionId: o.id } } }));
  const choose = (o: TripOption) =>
    change(
      api.PUT("/v1/households/{householdId}/trips/{tripId}", {
        params: { path },
        body: { name: trip.name, destination: trip.destination, startOn: trip.startOn, endOn: trip.endOn, notes: trip.notes, chosenOptionId: trip.chosenOptionId === o.id ? undefined : o.id },
      }),
    );

  const lead = leading(trip);
  const when = dates(trip);
  return (
    <Screen
      title={trip.name}
      overline={[trip.destination, when].filter(Boolean).join(" · ")}
      back={back}
      action={
        <button type="button" className="screen-action" aria-label={t("trips.edit")} onClick={() => setSheet("trip")}>
          <Icon name="edit" />
        </button>
      }
    >
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {trip.notes && <p className="trip-notes">{trip.notes}</p>}

      <section className="group" aria-label={t("trips.placesTitle")}>
        <div className="today-chores-head">
          <h2 className="label group-title">{t("trips.placesTitle")}</h2>
          <Button variant="plain" size="sm" onClick={() => setSheet("option")}>
            {t("trips.optionNew")}
          </Button>
        </div>
        {trip.options.length === 0 && <p className="caption muted">{t("trips.placesEmpty")}</p>}
        <div className="trip-options">
          {trip.options.map((o) => (
            <OptionCard
              key={o.id}
              option={o}
              household={household}
              me={me}
              chosen={trip.chosenOptionId === o.id}
              leads={lead?.id === o.id && !trip.chosenOptionId}
              onVote={() => vote(o)}
              onChoose={() => choose(o)}
              onEdit={() => setSheet(o)}
            />
          ))}
        </div>
      </section>

      <Packing householdId={household.id} trip={trip} onChange={change} />

      {sheet === "trip" && (
        <TripSheet
          householdId={household.id}
          trip={trip}
          onClose={() => setSheet(null)}
          onSaved={() => (setSheet(null), reload())}
          onDeleted={() => navigate("/trips", { replace: true })}
        />
      )}
      {(sheet === "option" || (sheet && typeof sheet === "object")) && (
        <OptionSheet
          householdId={household.id}
          trip={trip}
          option={typeof sheet === "object" ? sheet : undefined}
          onClose={() => setSheet(null)}
          onSaved={(next) => (setSheet(null), apply(next), reload())}
        />
      )}
    </Screen>
  );
}

// A place: its photos side by side, then who favours it and whether it is the one.
function OptionCard({ option, household, me, chosen, leads, onVote, onChoose, onEdit }: {
  option: TripOption;
  household: HouseholdDetail;
  me: User;
  chosen: boolean;
  leads: boolean;
  onVote: () => void;
  onChoose: () => void;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const mine = option.votes.includes(me.id);
  const voters = option.votes.map((v) => household.members.findIndex((m) => m.user.id === v)).filter((i) => i >= 0);
  const host = (() => {
    try {
      return option.url ? new URL(option.url).host.replace(/^www\./, "") : "";
    } catch {
      return "";
    }
  })();
  return (
    <article className={cx("hl-card trip-option", chosen && "trip-option-chosen")}>
      {option.photos.length > 0 && (
        <div className="trip-photos" tabIndex={0} aria-label={t("trips.photos")}>
          {option.photos.map((p) => (
            <a key={p.id} href={p.url} target="_blank" rel="noreferrer">
              <img src={p.url} alt="" loading="lazy" />
            </a>
          ))}
        </div>
      )}
      <div className="trip-option-head">
        <h3 className="hl-card-title">{option.title}</h3>
        <button type="button" className="screen-action" aria-label={t("trips.optionEdit")} onClick={onEdit}>
          <Icon name="edit" size="sm" />
        </button>
      </div>
      {(chosen || leads) && <span className="label trip-badge">{t(chosen ? "trips.chosen" : "trips.leads")}</span>}
      {option.notes && <p className="hl-card-body">{option.notes}</p>}
      {option.url && (
        <a className="callout trip-link" href={option.url} target="_blank" rel="noreferrer">
          {host || t("trips.link")}
        </a>
      )}
      <div className="trip-option-actions">
        <button type="button" className={cx("trip-vote", mine && "trip-vote-on")} aria-pressed={mine} onClick={onVote}>
          <Icon name="heart" size="sm" />
          {mine ? t("trips.voted") : t("trips.vote")}
        </button>
        {voters.length > 0 && (
          <span className="hl-pair" aria-label={t("trips.votes", { count: voters.length })}>
            {voters.map((i) => {
              const m = household.members[i];
              return m ? <ThreadAvatar key={m.user.id} name={m.user.name} src={m.user.avatarUrl} thread={threadFor(i)} size="sm" /> : null;
            })}
          </span>
        )}
        <Button variant={chosen ? "secondary" : "plain"} size="sm" onClick={onChoose}>
          {t(chosen ? "trips.unchoose" : "trips.choose")}
        </Button>
      </div>
    </article>
  );
}

// What to pack: tick it off, swipe it away, add more at the bottom.
function Packing({ householdId, trip, onChange }: { householdId: string; trip: Trip; onChange: (request: Promise<{ data?: Trip; error?: { message: string } }>) => Promise<void> }) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const base = { householdId, tripId: trip.id };
  const path = (itemId: string) => ({ ...base, itemId });
  const tick = (p: PackingItem) =>
    onChange(api.PUT("/v1/households/{householdId}/trips/{tripId}/packing/{itemId}", { params: { path: path(p.id) }, body: { text: p.text, done: !p.done } }));
  const remove = (p: PackingItem) => onChange(api.DELETE("/v1/households/{householdId}/trips/{tripId}/packing/{itemId}", { params: { path: path(p.id) } }));
  async function add(e: FormEvent) {
    e.preventDefault();
    const value = text.trim();
    if (!value) return;
    setText("");
    await onChange(api.POST("/v1/households/{householdId}/trips/{tripId}/packing", { params: { path: base }, body: { text: value } }));
  }
  return (
    <section className="group" aria-label={t("trips.packing")}>
      <div className="today-chores-head">
        <h2 className="label group-title">{t("trips.packing")}</h2>
        {trip.packing.length > 0 && <span className="caption muted">{t("trips.packed", { done: packed(trip), count: trip.packing.length })}</span>}
      </div>
      {trip.packing.length > 0 && (
        <div className="hl-list">
          {trip.packing.map((p) => (
            <Swipe key={p.id} end={{ label: t("trips.remove"), icon: "trash", tone: "danger", onAction: () => void remove(p) }}>
              <div className="entry-row">
                <button type="button" className={p.done ? "tick tick-done" : "tick"} role="checkbox" aria-checked={p.done} aria-label={p.text} onClick={() => tick(p)}>
                  {p.done && <Icon name="check" size="sm" />}
                </button>
                <ListRow title={<span className={cx(p.done && "trip-packed")}>{p.text}</span>} />
              </div>
            </Swipe>
          ))}
        </div>
      )}
      <form className="trip-add" onSubmit={add}>
        <input className="input" value={text} maxLength={100} onChange={(e) => setText(e.target.value)} placeholder={t("trips.packingPlaceholder")} enterKeyHint="done" aria-label={t("trips.packingPlaceholder")} />
        <Button type="submit" size="sm" disabled={!text.trim()}>
          {t("shopping.add")}
        </Button>
      </form>
    </section>
  );
}
