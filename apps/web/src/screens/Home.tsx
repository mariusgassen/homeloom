import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";
import { api, type Home as HomeState, type Light, type Reading, type Room, type Scene } from "../api/client";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { cx, Icon } from "../ui/Icon";
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

  useEffect(() => {
    let current = true;
    api
      .GET("/v1/households/{householdId}/home", { params: { path: { householdId } } })
      .catch(() => undefined)
      .then((res) => {
        if (!current) return;
        if (res?.data) setHome(res.data);
        else setUnavailable(res?.error?.message ?? t("home.failed"));
      });
    return () => {
      current = false;
    };
  }, [householdId, t]);

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

  if (unavailable) {
    return (
      <Screen title={t("tabs.home")}>
        <Card tone="sunken" title={t("home.offTitle")}>
          {unavailable}
        </Card>
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
    return (
      <section key={id ?? ""} className="room" aria-label={title ?? t("tabs.home")}>
        {(title || readings.length > 0) && (
          <header className="room-header">
            {title && <h2 className="headline">{title}</h2>}
            <Readings readings={readings} />
          </header>
        )}
        {lights.length > 0 && (
          <div className="room-lights">
            {lights.map((l) => (
              <button key={l.id} type="button" className={cx("light", l.on && "light-on")} role="switch" aria-checked={l.on} aria-label={l.name} disabled={disabled} onClick={() => toggle(l)}>
                <span className="light-name">{l.name}</span>
                <span className="caption">{l.on ? t("home.on") : t("home.off")}</span>
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
  return (
    <Screen title={t("tabs.home")}>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {loose.length > 0 && (
        <section className="group" aria-label={t("home.scenes")}>
          <h2 className="label group-title">{t("home.scenes")}</h2>
          <div className="scene-grid">
            {loose.map((s) => (
              <button key={s.id} type="button" className="scene" disabled={disabled} onClick={() => scene(s.id)} aria-pressed={active === s.id}>
                <Icon name="sparkle" />
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
      {empty && <Card tone="sunken" title={t("home.emptyTitle")} />}
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
          {number(r)}
        </span>
      ))}
      {alerts.map((r) => (
        <span key={r.id} className={cx("hl-chip", r.kind === "smoke" || r.kind === "moisture" ? "room-alarm" : "room-alert")} title={r.name}>
          {t(`home.reading.${r.kind}`)}
        </span>
      ))}
    </div>
  );
}
