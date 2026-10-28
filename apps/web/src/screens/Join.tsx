import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { api } from "../api/client";
import { useSession } from "../session";
import { Button } from "../ui/Button";

// An invite link survives the trip through sign-in in localStorage: login always lands on "/".
const key = "homeloom.invite";

export const pendingInvite = {
  get: () => {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  },
  set: (code: string) => {
    try {
      localStorage.setItem(key, code);
    } catch {
      // Private mode: the invite then works only when already signed in.
    }
  },
  clear: () => {
    try {
      localStorage.removeItem(key);
    } catch {
      // Nothing stored.
    }
  },
};

export function Join({ code }: { code: string }) {
  const { t } = useTranslation();
  const { session, reload } = useSession();
  const navigate = useNavigate();
  const [joined, setJoined] = useState(false);
  const [failed, setFailed] = useState<string | null>(null);
  const sent = useRef(false);

  useEffect(() => {
    if (sent.current) return;
    sent.current = true;
    api
      .POST("/v1/invites/{code}", { params: { path: { code } } })
      .catch(() => undefined)
      .then((res) => {
        pendingInvite.clear();
        if (res?.data) {
          setJoined(true);
          reload();
        } else {
          setFailed(res?.error?.message ?? t("join.failed"));
        }
      });
  }, [code, reload, t]);

  // Leave once the session has a household, so onboarding never flashes in between.
  const home = session.state === "signedIn" ? session.household?.id : undefined;
  useEffect(() => {
    if (joined && home) navigate("/", { replace: true });
  }, [joined, home, navigate]);

  if (!failed) {
    return (
      <div className="center">
        <p className="body muted">{t("join.joining")}</p>
      </div>
    );
  }
  return (
    <div className="center">
      <h1 className="title">{t("join.title")}</h1>
      <p className="callout error" role="alert">
        {failed}
      </p>
      <Button block onClick={() => navigate("/", { replace: true })}>
        {t("join.continue")}
      </Button>
    </div>
  );
}
