import { useTranslation } from "react-i18next";
import { Button } from "../ui/Button";

const knownErrors = ["login_failed", "auth_unavailable"] as const;

export function Welcome({ invited }: { invited?: boolean }) {
  const { t } = useTranslation();
  const code = new URLSearchParams(window.location.search).get("auth_error");
  const error = knownErrors.find((e) => e === code);
  return (
    <div className="center">
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

export function Unavailable({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="center">
      <h1 className="title">{t("unavailable.title")}</h1>
      <p className="body muted">{t("unavailable.body")}</p>
      <Button variant="secondary" block onClick={onRetry}>
        {t("unavailable.retry")}
      </Button>
    </div>
  );
}
