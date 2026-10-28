import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";
import { api, type Home as HomeState, type Light } from "../api/client";
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

// Home shows what Home Assistant lets Homeloom touch, live: scenes and lights. Each person
// connects their own Home Assistant account, so switches show up there under their name.
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
      (ok) => ok && setHome({ connected: false, scenes: [], lights: [] }),
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
        setHome((h) => h && { ...h, lights: h.lights.map((x) => (x.id === next.id ? next : x)) });
      }
      return res;
    });

  if (unavailable) {
    return (
      <Screen title={t("tabs.home")}>
        <Card tone="sunken" title={t("home.offTitle")}>
          {unavailable}
        </Card>
        <p className="caption muted">{t("home.whitelist")}</p>
      </Screen>
    );
  }

  const scenes = home?.scenes ?? [];
  return (
    <Screen title={t("tabs.home")} overline={home?.connected ? t("home.connected") : undefined}>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {scenes.length > 0 && (
        <section className="group" aria-label={t("home.scenes")}>
          <h2 className="label group-title">{t("home.scenes")}</h2>
          <div className="scene-grid">
            {scenes.map((s) => (
              <button key={s.id} type="button" className="scene" disabled={busy !== null} onClick={() => scene(s.id)} aria-pressed={active === s.id}>
                <Icon name="sparkle" />
                <span className="headline">{s.name}</span>
                {active === s.id && <span className="caption">{t("home.on")}</span>}
              </button>
            ))}
          </div>
        </section>
      )}
      {home && home.lights.length > 0 && (
        <section className="group" aria-label={t("home.lights")}>
          <h2 className="label group-title">{t("home.lights")}</h2>
          <div className="hl-list">
            {home.lights.map((l) => (
              <button key={l.id} type="button" className="hl-listrow" role="switch" aria-checked={l.on} disabled={busy !== null} onClick={() => toggle(l)}>
                <span className="hl-listrow-main">
                  <span className="hl-listrow-title">{l.name}</span>
                </span>
                <span className={cx("switch", l.on && "switch-on")} aria-hidden="true" />
              </button>
            ))}
          </div>
        </section>
      )}
      {home && !home.connected && (
        <Card
          tone="sunken"
          title={t("home.connectTitle")}
          footer={
            <Button size="sm" disabled={busy !== null} onClick={connect}>
              {t("home.connect")}
            </Button>
          }
        >
          {t("home.connectBody")}
        </Card>
      )}
      {home?.connected && scenes.length === 0 && home.lights.length === 0 && (
        <Card tone="sunken" title={t("home.emptyTitle")}>
          {t("home.emptyBody")}
        </Card>
      )}
      <p className="caption muted">{t("home.whitelist")}</p>
      {home?.connected && (
        <Button variant="plain" size="sm" className="home-disconnect" disabled={busy !== null} onClick={disconnect}>
          {t("home.disconnect")}
        </Button>
      )}
    </Screen>
  );
}
