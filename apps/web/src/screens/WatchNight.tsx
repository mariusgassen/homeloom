import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { api, type Scene, type WatchTitle } from "../api/client";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { Icon } from "../ui/Icon";
import { Mark } from "../ui/Mark";
import { Screen } from "./Screen";
import { Poster, useTonight } from "./Watch";

// Movie night: each of you swipes through the films you haven't seen; a film you both like is a
// match, which you can pick for tonight and set the room for.
export function WatchNight() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("movie night without a household");
  const householdId = session.household.id;
  const tonight = useTonight(householdId);
  const [deck, setDeck] = useState<WatchTitle[] | null>(null);
  const [matches, setMatches] = useState<WatchTitle[]>([]);
  const [partners, setPartners] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [fresh, setFresh] = useState<WatchTitle | null>(null);
  const [scenes, setScenes] = useState<Scene[]>([]);
  const [scene, setScene] = useState<string | null>(null);

  const load = useCallback(async () => {
    const res = await api.GET("/v1/households/{householdId}/watch/night", { params: { path: { householdId } } }).catch(() => undefined);
    if (res?.data) {
      setDeck(res.data.deck);
      setMatches(res.data.matches);
      setPartners(res.data.partners);
      setError(null);
    } else {
      setDeck([]);
      setError(res?.error?.message ?? t("watch.failed"));
    }
  }, [householdId, t]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- loads once on arrival
    void load();
    api
      .GET("/v1/households/{householdId}/home", { params: { path: { householdId } } })
      .catch(() => undefined)
      .then((res) => setScenes(res?.data?.scenes ?? []));
  }, [load, householdId]);

  async function swipe(title: WatchTitle, liked: boolean) {
    setDeck((d) => d?.filter((x) => x.id !== title.id) ?? null);
    const res = await api
      .POST("/v1/households/{householdId}/watch/night/swipes", { params: { path: { householdId } }, body: { titleId: title.id, liked } })
      .catch(() => undefined);
    if (!res?.data) {
      setDeck((d) => [title, ...(d ?? [])]);
      return setError(res?.error?.message ?? t("watch.failed"));
    }
    const match = res.data.match;
    if (match) {
      setMatches((m) => [match, ...m.filter((x) => x.id !== match.id)]);
      setFresh(match);
    }
  }

  async function restart() {
    const res = await api.DELETE("/v1/households/{householdId}/watch/night", { params: { path: { householdId } } }).catch(() => undefined);
    if (!res?.response.ok) return setError(t("watch.failed"));
    setFresh(null);
    await load();
  }

  async function activate(id: string) {
    const res = await api.POST("/v1/households/{householdId}/home/scenes/{sceneId}", { params: { path: { householdId, sceneId: id } } }).catch(() => undefined);
    if (res?.response.ok) setScene(id);
    else setError(t("home.failed"));
  }

  const back = (
    <Button variant="plain" size="sm" className="back" onClick={() => navigate("/watch")}>
      <span className="back-icon">
        <Icon name="chevron" size="sm" />
      </span>
      {t("watch.title")}
    </Button>
  );
  const top = deck?.[0];
  return (
    <Screen title={t("night.title")} back={back}>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {!deck && (
        <div className="loading-inline">
          <Mark size={40} weaving label={t("refresh.loading")} />
        </div>
      )}
      {fresh && (
        <section className="night-match hl-card" aria-live="polite">
          <span className="hl-eyebrow">{t("night.match")}</span>
          <h2 className="headline">{fresh.name}</h2>
          {tonight.pick?.titleId === fresh.id ? (
            <span className="caption muted">{t("watch.tonight.picked")}</span>
          ) : (
            <Button icon="film" onClick={() => tonight.choose(fresh)}>
              {t("watch.tonight.pick")}
            </Button>
          )}
          {scenes.length > 0 && (
            <div className="night-scenes" role="group" aria-label={t("night.scene")}>
              {scenes.map((s) => (
                <button key={s.id} type="button" className="hl-chip" aria-pressed={scene === s.id} onClick={() => activate(s.id)}>
                  {s.name}
                </button>
              ))}
            </div>
          )}
          <Button variant="plain" size="sm" onClick={() => setFresh(null)}>
            {t("night.keepGoing")}
          </Button>
        </section>
      )}
      {partners === 0 && <p className="caption muted">{t("night.alone")}</p>}
      {top && <SwipeCard key={top.id} householdId={householdId} title={top} onSwipe={(liked) => swipe(top, liked)} />}
      {deck && !top && !error && (
        <Empty
          title={t(matches.length > 0 ? "night.doneMatches" : "night.done")}
          action={
            <Button variant="secondary" size="sm" onClick={() => void load()}>
              {t("night.more")}
            </Button>
          }
        />
      )}
      {matches.length > 0 && (
        <section className="group" aria-label={t("night.matches")}>
          <h2 className="label group-title">{t("night.matches")}</h2>
          <div className="watch-grid">
            {matches.map((m) => (
              <button key={m.id} type="button" className="watch-title" onClick={() => setFresh(m)}>
                <Poster householdId={householdId} title={m} />
                <span className="watch-name">{m.name}</span>
              </button>
            ))}
          </div>
        </section>
      )}
      {(deck?.length ?? 0) + matches.length > 0 && (
        <Button variant="plain" size="sm" className="home-disconnect" onClick={restart}>
          {t("night.restart")}
        </Button>
      )}
    </Screen>
  );
}

const THRESHOLD = 90;

// One film. Drag it right to like and left to pass, or use the buttons.
function SwipeCard({ householdId, title, onSwipe }: { householdId: string; title: WatchTitle; onSwipe: (liked: boolean) => void }) {
  const { t } = useTranslation();
  const [dx, setDx] = useState(0);
  const [start, setStart] = useState<number | null>(null);
  const facts = [title.year, title.minutes && t("watch.minutes", { count: title.minutes }), title.genres.slice(0, 2).join(", ")].filter(Boolean).join(" · ");
  return (
    <div className="night-deck">
      <article
        className="night-card hl-card"
        style={{ transform: `translateX(${dx}px) rotate(${dx / 20}deg)`, transition: start === null ? "transform .2s" : "none" }}
        onPointerDown={(e) => {
          setStart(e.clientX);
          e.currentTarget.setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => start !== null && setDx(e.clientX - start)}
        onPointerUp={() => {
          const moved = dx;
          setStart(null);
          setDx(0);
          if (Math.abs(moved) >= THRESHOLD) onSwipe(moved > 0);
        }}
        onPointerCancel={() => {
          setStart(null);
          setDx(0);
        }}
      >
        <Poster householdId={householdId} title={title} />
        <h2 className="headline">{title.name}</h2>
        {facts && <p className="caption muted">{facts}</p>}
        {title.summary && <p className="night-summary">{title.summary}</p>}
      </article>
      <div className="night-actions">
        <Button variant="secondary" icon="close" aria-label={t("night.pass")} onClick={() => onSwipe(false)} />
        <Button icon="heart" aria-label={t("night.like")} onClick={() => onSwipe(true)} />
      </div>
    </div>
  );
}
