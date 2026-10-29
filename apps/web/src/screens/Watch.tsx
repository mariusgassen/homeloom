import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router";
import { api, type WatchLibrary, type WatchShelf, type WatchTitle } from "../api/client";
import { useRefresh } from "../refresh";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Empty } from "../ui/Empty";
import { cx, Icon } from "../ui/Icon";
import { Mark } from "../ui/Mark";
import { Screen } from "./Screen";

function useHouseholdId() {
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("watch without a household");
  return session.household.id;
}

const posterURL = (householdId: string, titleId: string) =>
  `/v1/households/${encodeURIComponent(householdId)}/watch/titles/${encodeURIComponent(titleId)}/poster`;

// Movies and series from the libraries you can watch: your own and those friends share. Each
// person connects their own account, so "not seen yet" is about them.
export function Watch() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const householdId = useHouseholdId();
  const [params] = useSearchParams();
  const connectError = params.get("watch_error");
  const [shelf, setShelf] = useState<WatchShelf | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(connectError && t(`watch.error.${connectError}`, { defaultValue: t("watch.failed") }));
  const [busy, setBusy] = useState(false);
  const [library, setLibrary] = useState<string | null>(null);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/watch", { params: { path: { householdId } } })
      .catch(() => undefined)
      .then((res) => {
        if (!current) return;
        if (res?.data) setShelf(res.data);
        setUnavailable(res?.data ? null : (res?.error?.message ?? t("watch.failed")));
      });
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, t, refreshed, track]);

  // Signing in happens at Plex, as a full navigation; it comes back to /watch.
  async function connect() {
    setBusy(true);
    setError(null);
    const res = await api.POST("/v1/households/{householdId}/watch/connection", { params: { path: { householdId } } }).catch(() => undefined);
    if (res?.data) return window.location.assign(res.data.url);
    setBusy(false);
    setError(res?.error?.message ?? t("watch.failed"));
  }

  async function disconnect() {
    setBusy(true);
    const res = await api.DELETE("/v1/households/{householdId}/watch/connection", { params: { path: { householdId } } }).catch(() => undefined);
    setBusy(false);
    if (!res?.response.ok) return setError(t("watch.failed"));
    setShelf({ connected: false, libraries: [] });
  }

  const back = (
    <Button variant="plain" size="sm" className="back" onClick={() => navigate("/us")}>
      <span className="back-icon">
        <Icon name="chevron" size="sm" />
      </span>
      {t("tabs.us")}
    </Button>
  );

  if (unavailable) {
    return (
      <Screen title={t("watch.title")} back={back}>
        <Empty title={t("watch.offTitle")}>{unavailable}</Empty>
      </Screen>
    );
  }

  const libraries = shelf?.libraries ?? [];
  const chosen = libraries.find((l) => l.id === library) ?? libraries[0];
  return (
    <Screen title={t("watch.title")} back={back}>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {!shelf && (
        <div className="loading-inline">
          <Mark size={40} weaving label={t("refresh.loading")} />
        </div>
      )}
      {shelf && !shelf.connected && (
        <Card
          tone="sunken"
          title={t("watch.connectTitle")}
          footer={
            <Button size="sm" disabled={busy} onClick={connect}>
              {t("watch.connect")}
            </Button>
          }
        >
          {t("watch.connectBody")}
        </Card>
      )}
      {shelf?.connected && libraries.length === 0 && <Empty title={t("watch.emptyTitle")}>{t("watch.emptyBody")}</Empty>}
      {libraries.length > 1 && <LibraryPicker libraries={libraries} chosen={chosen} onPick={setLibrary} />}
      {chosen && <Library key={chosen.id} householdId={householdId} library={chosen} />}
      {shelf?.connected && (
        <Button variant="plain" size="sm" className="home-disconnect" disabled={busy} onClick={disconnect}>
          {t("watch.disconnect")}
        </Button>
      )}
    </Screen>
  );
}

function LibraryPicker({ libraries, chosen, onPick }: { libraries: WatchLibrary[]; chosen?: WatchLibrary; onPick: (id: string) => void }) {
  const { t } = useTranslation();
  return (
    <div className="watch-libraries" role="tablist" aria-label={t("watch.libraries")}>
      {libraries.map((l) => (
        <button key={l.id} type="button" role="tab" className={cx("hl-chip", "watch-library")} aria-selected={l.id === chosen?.id} onClick={() => onPick(l.id)}>
          {l.name}
          {!l.owned && <span className="caption"> · {l.server}</span>}
        </button>
      ))}
    </div>
  );
}

// One library: its titles as posters, the unseen ones only if asked, and a random pick from them.
function Library({ householdId, library }: { householdId: string; library: WatchLibrary }) {
  const { t } = useTranslation();
  const [titles, setTitles] = useState<WatchTitle[] | null>(null);
  const [failed, setFailed] = useState<string | null>(null);
  const [unseen, setUnseen] = useState(false);
  const [open, setOpen] = useState<WatchTitle | null>(null);

  useEffect(() => {
    let current = true;
    api
      .GET("/v1/households/{householdId}/watch/libraries/{libraryId}/titles", { params: { path: { householdId, libraryId: library.id } } })
      .catch(() => undefined)
      .then((res) => {
        if (!current) return;
        if (res?.data) setTitles(res.data);
        else setFailed(res?.error?.message ?? t("watch.failed"));
      });
    return () => {
      current = false;
    };
  }, [householdId, library.id, t]);

  const shown = useMemo(() => (unseen ? titles?.filter((x) => !x.watched) : titles) ?? [], [titles, unseen]);
  const surprise = () => setOpen(shown[Math.floor(Math.random() * shown.length)] ?? null);

  if (failed) {
    return (
      <p className="callout error" role="alert">
        {failed}
      </p>
    );
  }
  if (!titles) {
    return (
      <div className="loading-inline">
        <Mark size={40} weaving label={t("refresh.loading")} />
      </div>
    );
  }
  return (
    <section className="group" aria-label={library.name}>
      <div className="watch-bar">
        <button type="button" className="hl-chip watch-filter" aria-pressed={unseen} onClick={() => setUnseen((u) => !u)}>
          {t("watch.unseen")}
        </button>
        <span className="caption muted">{t("watch.count", { count: shown.length })}</span>
        <Button size="sm" disabled={shown.length === 0} onClick={surprise}>
          <Icon name="sparkle" size="sm" />
          {t("watch.surprise")}
        </Button>
      </div>
      {shown.length === 0 ? (
        <Empty title={t("watch.allSeen")} />
      ) : (
        <div className="watch-grid">
          {shown.map((x) => (
            <button key={x.id} type="button" className="watch-title" onClick={() => setOpen(x)}>
              <Poster householdId={householdId} title={x} />
              <span className="watch-name">{x.name}</span>
              {x.year ? <span className="caption muted">{x.year}</span> : null}
            </button>
          ))}
        </div>
      )}
      {open && <TitleSheet householdId={householdId} title={open} onClose={() => setOpen(null)} onAnother={shown.length > 1 ? surprise : undefined} />}
    </section>
  );
}

function Poster({ householdId, title }: { householdId: string; title: WatchTitle }) {
  const [missing, setMissing] = useState(false);
  if (missing) return <span className="watch-poster watch-poster-none" aria-hidden="true">{title.name.slice(0, 1)}</span>;
  return <img className="watch-poster" src={posterURL(householdId, title.id)} alt="" loading="lazy" onError={() => setMissing(true)} />;
}

function TitleSheet({ householdId, title, onClose, onAnother }: { householdId: string; title: WatchTitle; onClose: () => void; onAnother?: () => void }) {
  const { t, i18n } = useTranslation();
  const facts = [
    title.year,
    title.kind === "show" && title.episodes ? t("watch.episodes", { count: title.episodes }) : title.minutes && t("watch.minutes", { count: title.minutes }),
    title.rating && `★ ${new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(title.rating)}`,
    title.contentRating,
  ].filter(Boolean);
  return (
    <div className="sheet-backdrop" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className="sheet" role="dialog" aria-modal="true" aria-label={title.name}>
        <div className="sheet-grabber" aria-hidden="true" />
        <div className="sheet-bar">
          <Button variant="plain" onClick={onClose}>
            {t("entries.done")}
          </Button>
          <h2 className="headline sheet-title">{title.name}</h2>
        </div>
        <div className="watch-detail">
          <Poster householdId={householdId} title={title} />
          <div className="watch-facts">
            {facts.length > 0 && <p className="caption muted">{facts.join(" · ")}</p>}
            {title.genres.length > 0 && <p className="caption">{title.genres.join(", ")}</p>}
            {title.watched && <span className="hl-chip">{t("watch.seen")}</span>}
          </div>
        </div>
        {title.summary && <p className="callout">{title.summary}</p>}
        {onAnother && (
          <Button variant="secondary" block onClick={onAnother}>
            {t("watch.another")}
          </Button>
        )}
      </div>
    </div>
  );
}
