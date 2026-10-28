import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../api/client";
import { useSession } from "../session";
import { Button } from "../ui/Button";

export function Onboarding() {
  const { t } = useTranslation();
  const { reload } = useSession();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setFailed(false);
    const res = await api.POST("/v1/households", { body: { name: name.trim() } }).catch(() => undefined);
    setBusy(false);
    if (res?.data) reload();
    else setFailed(true);
  }

  return (
    <form className="center" onSubmit={submit}>
      <h1 className="large-title">{t("onboarding.title")}</h1>
      <p className="body muted">{t("onboarding.body")}</p>
      <label className="field">
        <span className="label">{t("onboarding.name")}</span>
        <input
          className="input"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t("onboarding.placeholder")}
          maxLength={100}
          required
          autoFocus
        />
      </label>
      {failed && (
        <p className="callout error" role="alert">
          {t("onboarding.failed")}
        </p>
      )}
      <Button type="submit" block disabled={busy || !name.trim()}>
        {t("onboarding.create")}
      </Button>
    </form>
  );
}
