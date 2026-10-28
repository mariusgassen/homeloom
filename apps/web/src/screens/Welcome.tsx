import { Component, useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useOnline } from "../refresh";
import { Button } from "../ui/Button";
import { Mark } from "../ui/Mark";

const knownErrors = ["login_failed", "auth_unavailable"] as const;

export function Welcome({ invited }: { invited?: boolean }) {
  const { t } = useTranslation();
  const code = new URLSearchParams(window.location.search).get("auth_error");
  const error = knownErrors.find((e) => e === code);
  return (
    <div className="center center-state">
      <Mark size={88} />
      <h1 className="large-title">Homeloom</h1>
      <p className="body muted">{t(invited ? "join.signIn" : "app.tagline")}</p>
      {error && (
        <p className="callout error" role="alert">
          {t(`auth.error.${error}`)}
        </p>
      )}
      {/* A full navigation, not fetch: the API redirects on to authentik. */}
      <Button block onClick={() => window.location.assign("/auth/login")}>
        {t("auth.signIn")}
      </Button>
    </div>
  );
}

// Starting up: the mark weaves, after a beat so a fast start doesn't flash.
export function Loading() {
  const { t } = useTranslation();
  return (
    <div className="center center-state loading">
      <Mark size={72} weaving label={t("refresh.loading")} />
    </div>
  );
}

// The API can't be reached. Offline says so, and tries again by itself once the phone is back online.
export function Unavailable({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  const online = useOnline();
  const wasOnline = useRef(online);
  useEffect(() => {
    if (online && !wasOnline.current) onRetry();
    wasOnline.current = online;
  }, [online, onRetry]);
  const key = online ? "unavailable" : "unavailable.offline";
  return (
    <div className="center center-state">
      <Mark size={64} />
      <h1 className="title">{t(`${key}.title`)}</h1>
      <p className="body muted">{t(`${key}.body`)}</p>
      <Button variant="secondary" block onClick={onRetry}>
        {t("unavailable.retry")}
      </Button>
    </div>
  );
}

// Crashed catches what the screens didn't, so a bug shows the mark and a way back, not a blank page.
export class Crashed extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? <CrashedPage /> : this.props.children;
  }
}

function CrashedPage() {
  const { t } = useTranslation();
  return (
    <div className="center center-state">
      <Mark size={64} />
      <h1 className="title">{t("crashed.title")}</h1>
      <p className="body muted">{t("crashed.body")}</p>
      <Button variant="secondary" block onClick={() => window.location.reload()}>
        {t("crashed.reload")}
      </Button>
    </div>
  );
}
