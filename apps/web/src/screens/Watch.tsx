import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router";
import { api, type WatchLibrary, type WatchShelf, type WatchTitle } from "../api/client";
import { useRefresh } from "../refresh";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Empty } from "../ui/Empty";
import { FormBlock, FormGroup, Pills, Segmented, SwitchRow } from "../ui/Form";
import { cx, Icon } from "../ui/Icon";
import { Mark } from "../ui/Mark";
import { Sheet } from "../ui/Sheet";
import { Screen } from "./Screen";
import { WatchIdeas } from "./WatchIdeas";

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
  const [choosing, setChoosing] = useState(false);
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

  // Shown right away; put back if it couldn't be saved.
  async function include(l: WatchLibrary, included: boolean) {
    const set = (v: boolean) => setShelf((s) => s && { ...s, libraries: s.libraries.map((x) => (x.id === l.id ? { ...x, included: v } : x)) });
    set(included);
    setError(null);
    const res = await api
      .PUT("/v1/households/{householdId}/watch/libraries/{libraryId}", { params: { path: { householdId, libraryId: l.id } }, body: { included } })
      .catch(() => undefined);
    if (res?.response.ok) return;
    set(!included);
    setError(res?.error?.message ?? t("watch.failed"));
  }

  const back = (
    <Button variant="plain" size="sm" className="back" onClick={() => navigate("/")}>
      <span className="back-icon">
        <Icon name="chevron" size="sm" />
      </span>
      {t("tabs.today")}
    </Button>
  );

  if (unavailable) {
    return (
      <Screen title={t("watch.title")} back={back}>
        <WatchIdeas householdId={householdId} />
        <Empty title={t("watch.offTitle")}>{unavailable}</Empty>
      </Screen>
    );
  }

  const all = shelf?.libraries ?? [];
  const libraries = all.filter((l) => l.included);
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
      {shelf && <WatchIdeas householdId={householdId} />}
      {shelf?.connected && all.length === 0 && <Empty title={t("watch.emptyTitle")}>{t("watch.emptyBody")}</Empty>}
      {all.length > 0 && libraries.length === 0 && (
        <Empty
          title={t("watch.noneChosenTitle")}
          action={
            <Button variant="secondary" size="sm" onClick={() => setChoosing(true)}>
              {t("watch.choose")}
            </Button>
          }
        />
      )}
      {libraries.length > 0 && <LibraryPicker libraries={libraries} chosen={chosen} onPick={setLibrary} onChoose={() => setChoosing(true)} />}
      {chosen && <Library key={chosen.id} householdId={householdId} library={chosen} />}
      {shelf?.connected && (
        <Button variant="plain" size="sm" className="home-disconnect" disabled={busy} onClick={disconnect}>
          {t("watch.disconnect")}
        </Button>
      )}
      {choosing && <ChooseLibraries libraries={all} onInclude={include} onClose={() => setChoosing(false)} />}
    </Screen>
  );
}

function LibraryPicker({ libraries, chosen, onPick, onChoose }: { libraries: WatchLibrary[]; chosen?: WatchLibrary; onPick: (id: string) => void; onChoose: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="watch-libraries" role="tablist" aria-label={t("watch.libraries")}>
      {libraries.map((l) => (
        <button key={l.id} type="button" role="tab" className={cx("hl-chip", "watch-library")} aria-selected={l.id === chosen?.id} onClick={() => onPick(l.id)}>
          {l.name}
          {!l.owned && <span className="caption"> · {l.server}</span>}
        </button>
      ))}
      <button type="button" className={cx("hl-chip", "watch-library", "watch-choose")} onClick={onChoose}>
        {t("watch.choose")}
      </button>
    </div>
  );
}

// Which libraries count: your own choice, saved as you flip each one. New ones are on until you turn them off.
function ChooseLibraries({ libraries, onInclude, onClose }: { libraries: WatchLibrary[]; onInclude: (l: WatchLibrary, included: boolean) => void; onClose: () => void }) {
  const { t } = useTranslation();
  const groups = [libraries.filter((l) => l.owned), libraries.filter((l) => !l.owned)].filter((g) => g.length > 0);
  return (
    <Sheet
      title={t("watch.libraries")}
      onClose={onClose}
      trailing={
        <Button variant="plain" onClick={onClose}>
          {t("entries.done")}
        </Button>
      }
    >
      <p className="caption muted sheet-hint">{t("watch.chooseHint")}</p>
      {groups.map((g) => (
        <FormGroup key={g[0]?.owned ? "own" : "shared"}>
          {g.map((l) => (
            <SwitchRow
              key={l.id}
              label={l.owned ? l.name : `${l.name} · ${l.server}`}
              checked={l.included}
              onChange={(v) => onInclude(l, v)}
            />
          ))}
        </FormGroup>
      ))}
    </Sheet>
  );
}

type Filters = { query: string; unseen: boolean; genre: string; length: Length };
type Length = "" | "90" | "120";
const noFilters: Filters = { query: "", unseen: false, genre: "", length: "" };

// Case and accents don't matter when searching: "amelie" finds "Amélie".
const fold = (s: string) => s.normalize("NFD").replace(/\p{Diacritic}/gu, "").toLocaleLowerCase();

function matches(x: WatchTitle, f: Filters) {
  if (f.unseen && x.watched) return false;
  if (f.genre && !x.genres.includes(f.genre)) return false;
  if (f.length && !(x.minutes && x.minutes <= Number(f.length))) return false;
  return !f.query.trim() || fold(x.name).includes(fold(f.query.trim()));
}

// One library: its titles as posters, the unseen ones only if asked, and a random pick from them.
function Library({ householdId, library }: { householdId: string; library: WatchLibrary }) {
  const { t } = useTranslation();
  const [titles, setTitles] = useState<WatchTitle[] | null>(null);
  const [failed, setFailed] = useState<string | null>(null);
  const [filters, setFilters] = useState<Filters>(noFilters);
  const [filtering, setFiltering] = useState(false);
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

  const shown = useMemo(() => titles?.filter((x) => matches(x, filters)) ?? [], [titles, filters]);
  // The library's genres, most common first.
  const genres = useMemo(() => {
    const counts = new Map<string, number>();
    for (const x of titles ?? []) for (const g of x.genres) counts.set(g, (counts.get(g) ?? 0) + 1);
    return [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([g]) => g);
  }, [titles]);
  const active = [filters.unseen, filters.genre, filters.length].filter(Boolean).length;
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
        <input
          className="input watch-search"
          type="search"
          value={filters.query}
          onChange={(e) => setFilters((f) => ({ ...f, query: e.target.value }))}
          placeholder={t("watch.search")}
          aria-label={t("watch.search")}
          enterKeyHint="search"
        />
        <button type="button" className="hl-chip watch-filter" aria-pressed={active > 0} onClick={() => setFiltering(true)}>
          {active > 0 ? t("watch.filtered", { count: active }) : t("watch.filter")}
        </button>
      </div>
      <div className="watch-bar">
        <span className="caption muted">{t("watch.count", { count: shown.length })}</span>
        <Button size="sm" disabled={shown.length === 0} onClick={surprise}>
          <Icon name="sparkle" size="sm" />
          {t("watch.surprise")}
        </Button>
      </div>
      {shown.length === 0 ? (
        <Empty
          title={t(filters.query.trim() || filters.genre || filters.length ? "watch.noMatch" : "watch.allSeen")}
          action={
            (filters.query || active > 0) && (
              <Button variant="secondary" size="sm" onClick={() => setFilters(noFilters)}>
                {t("watch.reset")}
              </Button>
            )
          }
        />
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
      {filtering && <FilterSheet kind={library.kind} genres={genres} filters={filters} onChange={setFilters} onClose={() => setFiltering(false)} />}
    </section>
  );
}

// Narrow a library down; each choice applies right away, so the count behind the sheet follows.
function FilterSheet({ kind, genres, filters, onChange, onClose }: { kind: WatchLibrary["kind"]; genres: string[]; filters: Filters; onChange: (f: Filters) => void; onClose: () => void }) {
  const { t } = useTranslation();
  const set = (patch: Partial<Filters>) => onChange({ ...filters, ...patch });
  return (
    <Sheet
      title={t("watch.filter")}
      onClose={onClose}
      leading={
        <Button variant="plain" onClick={() => onChange({ ...noFilters, query: filters.query })}>
          {t("watch.reset")}
        </Button>
      }
      trailing={
        <Button variant="plain" onClick={onClose}>
          {t("entries.done")}
        </Button>
      }
    >
      <FormGroup>
        <SwitchRow label={t("watch.unseen")} checked={filters.unseen} onChange={(unseen) => set({ unseen })} />
        {kind === "movie" && (
          <FormBlock label={t("watch.length")}>
            <Segmented
              label={t("watch.length")}
              value={filters.length}
              options={(["", "90", "120"] as const).map((v) => ({ value: v, label: t(`watch.lengths.${v || "any"}`) }))}
              onChange={(length) => set({ length })}
            />
          </FormBlock>
        )}
        {genres.length > 0 && (
          <FormBlock label={t("watch.genre")}>
            <Pills label={t("watch.genre")} value={filters.genre} options={[{ value: "", label: t("watch.allGenres") }, ...genres.map((g) => ({ value: g, label: g }))]} onChange={(genre) => set({ genre })} />
          </FormBlock>
        )}
      </FormGroup>
    </Sheet>
  );
}

export function Poster({ householdId, title }: { householdId: string; title: WatchTitle }) {
  const [missing, setMissing] = useState(false);
  if (missing) return <span className="watch-poster watch-poster-none" aria-hidden="true">{title.name.slice(0, 1)}</span>;
  return <img className="watch-poster" src={posterURL(householdId, title.id)} alt="" loading="lazy" onError={() => setMissing(true)} />;
}

export function TitleSheet({ householdId, title, onClose, onAnother }: { householdId: string; title: WatchTitle; onClose: () => void; onAnother?: () => void }) {
  const { t, i18n } = useTranslation();
  const facts = [
    title.year,
    title.kind === "show" && title.episodes ? t("watch.episodes", { count: title.episodes }) : title.minutes && t("watch.minutes", { count: title.minutes }),
    title.rating && `★ ${new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(title.rating)}`,
    title.contentRating,
  ].filter(Boolean);
  return (
    <Sheet
      title={title.name}
      onClose={onClose}
      leading={
        <Button variant="plain" onClick={onClose}>
          {t("entries.done")}
        </Button>
      }
    >
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
    </Sheet>
  );
}
