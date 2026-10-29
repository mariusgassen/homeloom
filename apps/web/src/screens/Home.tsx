import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";
import { api, type Home as HomeState, type Light, type Reading, type Room, type Scene } from "../api/client";
import { useRefresh } from "../refresh";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Empty } from "../ui/Empty";
import { cx, Icon, type IconName } from "../ui/Icon";
import { Mark } from "../ui/Mark";
import { Screen } from "./Screen";

function useHouseholdId() {
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household) throw new Error("home without a household");
  return session.household.id;
}

const nothing: HomeState = { connected: false, rooms: [], scenes: [], lights: [], readings: [] };

// Home shows the home room by room, live: scenes and lights to switch, and readings such as the
// temperature or an open window. Each person connects their own Home Assistant account, so
// switches show up there under their name.
export function Home() {
  const { t } = useTranslation();
  const householdId = useHouseholdId();
  const [params] = useSearchParams();
  const connectError = params.get("home_error");
  const [home, setHome] = useState<HomeState | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(connectError && t(`home.error.${connectError}`, { defaultValue: t("home.failed") }));
  const [busy, setBusy] = useState<string | null>(null);
  const [active, setActive] = useState<string | null>(null);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/home", { params: { path: { householdId } } })
      .catch(() => undefined)
      .then((res) => {
        if (!current) return;
        if (res?.data) setHome(res.data);
        setUnavailable(res?.data ? null : (res?.error?.message ?? t("home.failed")));
      });
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, t, refreshed, track]);

  // run sends one switch; id marks which tile is busy and, on success, which scene is on.
  async function run(id: string, send: () => Promise<{ error?: { message: string } } | undefined>) {
    setBusy(id);
    setError(null);
    const res = await send().catch(() => undefined);
    setBusy(null);
    if (!res || res.error) {
      setError(res?.error?.message ?? t("home.failed"));
      return false;
    }
    return true;
  }

  // Signing in happens at Home Assistant, as a full navigation; it comes back to /home.
  const connect = () =>
    run("connect", async () => {
      const res = await api.POST("/v1/households/{householdId}/home/connection", { params: { path: { householdId } } });
      if (res.data) window.location.assign(res.data.url);
      return res;
    });
  const disconnect = () =>
    run("disconnect", () => api.DELETE("/v1/households/{householdId}/home/connection", { params: { path: { householdId } } })).then(
      (ok) => ok && setHome(nothing),
    );

  const scene = (sceneId: string) =>
    run(sceneId, () => api.POST("/v1/households/{householdId}/home/scenes/{sceneId}", { params: { path: { householdId, sceneId } } })).then(
      (ok) => ok && setActive(sceneId),
    );
  const toggle = (l: Light) =>
    run(l.id, async () => {
      const res = await api.POST("/v1/households/{householdId}/home/lights/{lightId}/toggle", { params: { path: { householdId, lightId: l.id } } });
      if (res.data) {
        const next = res.data;
        setHome((h) => h && { ...h, lights: h.lights.map((x) => (x.id === next.id ? { ...x, on: next.on } : x)) });
      }
      return res;
    });
  // All off switches the lit lights one after another, so each shows off as soon as it is.
  const allOff = () =>
    run("all", async () => {
      for (const l of home?.lights.filter((x) => x.on) ?? []) {
        const res = await api.POST("/v1/households/{householdId}/home/lights/{lightId}/toggle", { params: { path: { householdId, lightId: l.id } } });
        if (!res.data) return res;
        const next = res.data;
        setHome((h) => h && { ...h, lights: h.lights.map((x) => (x.id === next.id ? { ...x, on: next.on } : x)) });
      }
      return {};
    });

  if (unavailable) {
    return (
      <Screen title={t("tabs.home")}>
        <Empty title={t("home.offTitle")}>{unavailable}</Empty>
      </Screen>
    );
  }

  const disabled = busy !== null;
  const panel = (room: Room | undefined) => {
    const id = room?.id;
    const scenes = home?.scenes.filter((s) => s.room === id) ?? [];
    const lights = home?.lights.filter((l) => l.room === id) ?? [];
    const readings = home?.readings.filter((r) => r.room === id) ?? [];
    // Scenes outside any room have their own section above.
    if (!room && lights.length === 0 && readings.length === 0) return null;
    if (lights.length + readings.length + scenes.length === 0) return null;
    const title = room?.name ?? (home?.rooms.length ? t("home.other") : undefined);
    const lit = lights.filter((l) => l.on).length;
    return (
      <section key={id ?? ""} className={cx("room", lit > 0 && "room-lit")} aria-label={title ?? t("tabs.home")}>
        {title && (
          <header className="room-header">
            <span className="room-icon">
              <Icon name={roomIcon(title)} />
            </span>
            <span className="room-title">
              <h2 className="headline">{title}</h2>
              {lit > 0 && <span className="caption room-lit-count">{t("home.roomOn", { count: lit })}</span>}
            </span>
          </header>
        )}
        <Readings readings={readings} />
        {lights.length > 0 && (
          <div className="room-lights">
            {lights.map((l) => (
              <button key={l.id} type="button" className={cx("light", l.on && "light-on")} role="switch" aria-checked={l.on} aria-label={l.name} disabled={disabled} onClick={() => toggle(l)}>
                <span className="light-bulb">
                  <Icon name="bulb" />
                </span>
                <span className="light-text">
                  <span className="light-name">{l.name}</span>
                  <span className="caption">{l.on ? t("home.on") : t("home.off")}</span>
                </span>
              </button>
            ))}
          </div>
        )}
        {room && scenes.length > 0 && <Scenes scenes={scenes} active={active} disabled={disabled} onPick={scene} />}
      </section>
    );
  };

  const loose = home?.scenes.filter((s) => !s.room) ?? [];
  const empty = home?.connected && home.scenes.length + home.lights.length + home.readings.length === 0;
  const lit = home?.lights.filter((l) => l.on).length ?? 0;
  return (
    <Screen title={t("tabs.home")}>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {home && home.lights.length > 0 && (
        <div className={cx("home-summary", lit > 0 && "home-summary-lit")}>
          <span className="home-summary-bulb">
            <Icon name="bulb" />
          </span>
          <span className="headline">{lit > 0 ? t("home.lightsOn", { count: lit }) : t("home.lightsOff")}</span>
          {lit > 0 && (
            <Button size="sm" variant="secondary" disabled={disabled} onClick={allOff}>
              {t("home.allOff")}
            </Button>
          )}
        </div>
      )}
      {!home && (
        <div className="loading-inline">
          <Mark size={40} weaving label={t("refresh.loading")} />
        </div>
      )}
      {loose.length > 0 && (
        <section className="group" aria-label={t("home.scenes")}>
          <h2 className="label group-title">{t("home.scenes")}</h2>
          <div className="scene-grid">
            {loose.map((s) => (
              <button key={s.id} type="button" className="scene" disabled={disabled} onClick={() => scene(s.id)} aria-pressed={active === s.id}>
                <span className="scene-icon">
                  <Icon name="sparkle" />
                </span>
                <span className="headline">{s.name}</span>
                {active === s.id && <span className="caption">{t("home.on")}</span>}
              </button>
            ))}
          </div>
        </section>
      )}
      {home?.rooms.map((r) => panel(r))}
      {panel(undefined)}
      {home && !home.connected && (
        <Card
          tone="sunken"
          title={t("home.connectTitle")}
          footer={
            <Button size="sm" disabled={disabled} onClick={connect}>
              {t("home.connect")}
            </Button>
          }
        >
          {t("home.connectBody")}
        </Card>
      )}
      {empty && <Empty title={t("home.emptyTitle")} />}
      {home?.connected && (
        <Button variant="plain" size="sm" className="home-disconnect" disabled={disabled} onClick={disconnect}>
          {t("home.disconnect")}
        </Button>
      )}
    </Screen>
  );
}

function Scenes({ scenes, active, disabled, onPick }: { scenes: Scene[]; active: string | null; disabled: boolean; onPick: (id: string) => void }) {
  return (
    <div className="room-scenes">
      {scenes.map((s) => (
        <button key={s.id} type="button" className="room-scene" disabled={disabled} onClick={() => onPick(s.id)} aria-pressed={active === s.id}>
          <Icon name="sparkle" />
          {s.name}
        </button>
      ))}
    </div>
  );
}

// A room's icon, guessed from its name in English or German.
const roomIcons: [RegExp, IconName][] = [
  [/kitchen|küche|kueche|dining|essen/i, "cook"],
  [/living|wohn|lounge/i, "sofa"],
  [/bed|schlaf|kinder|nursery|guest|gäste|gaeste/i, "bed"],
  [/bath|bad|wc|toilet|dusch|shower/i, "bath"],
  [/office|study|büro|buero|arbeit/i, "desk"],
  [/garden|garten|balcon|balkon|terrace|terrasse|patio|outside|außen|aussen/i, "leaf"],
  [/hall|flur|diele|entr|eingang|stair|treppe/i, "door"],
];
function roomIcon(name: string): IconName {
  return roomIcons.find(([re]) => re.test(name))?.[1] ?? "today";
}

const readingIcons: Partial<Record<Reading["kind"], IconName>> = { temperature: "thermometer", humidity: "drop", window: "window", door: "door", garage_door: "door" };

function ReadingIcon({ kind }: { kind: Reading["kind"] }) {
  const name = readingIcons[kind];
  return name ? <Icon name={name} size="sm" /> : null;
}

// Readings shows one value per measurement (the first by name) and only the windows, doors and
// alarms that are open or detected, so a quiet room stays quiet.
function Readings({ readings }: { readings: Reading[] }) {
  const { t, i18n } = useTranslation();
  const seen = new Set<string>();
  const values = readings.filter((r) => r.value !== undefined && !seen.has(r.kind) && seen.add(r.kind));
  const alerts = readings.filter((r) => r.on);
  if (values.length + alerts.length === 0) return null;
  const number = (r: Reading) =>
    new Intl.NumberFormat(i18n.language, { maximumFractionDigits: r.kind === "temperature" ? 1 : 0 }).format(r.value ?? 0) + (r.unit ? ` ${r.unit}` : "");
  return (
    <div className="room-readings">
      {values.map((r) => (
        <span key={r.id} className="room-value" title={r.name}>
          <ReadingIcon kind={r.kind} />
          {number(r)}
        </span>
      ))}
      {alerts.map((r) => (
        <span key={r.id} className={cx("room-value", r.kind === "smoke" || r.kind === "moisture" ? "room-alarm" : "room-alert")} title={r.name}>
          <ReadingIcon kind={r.kind} />
          {t(`home.reading.${r.kind}`)}
        </span>
      ))}
    </div>
  );
}
