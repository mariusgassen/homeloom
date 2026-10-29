import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type HouseholdDetail, type Moment, type User, type Visibility } from "../api/client";
import { shrink, today, uploadPhoto } from "../moments";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";

// Keep a moment: what happened, the day, a photo if there is one, and who sees it. With a moment it
// edits that one instead.
export function MomentSheet({ household, me, moment, onClose, onSaved }: {
  household: HouseholdDetail;
  me: User;
  moment?: Moment;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState(moment?.text ?? "");
  const [day, setDay] = useState(moment?.day ?? today());
  const [visibility, setVisibility] = useState<Visibility>(moment?.visibility === "private" ? "private" : "shared");
  // undefined keeps the photo as it is, null removes it, a Blob replaces it.
  const [photo, setPhoto] = useState<Blob | null | undefined>(undefined);
  const [preview, setPreview] = useState<string | undefined>(moment?.photoUrl);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A moment saved before its photo failed is changed, not made again, on the next try.
  const saved = useRef<Moment | undefined>(moment);
  const textRef = useRef<HTMLTextAreaElement>(null);
  const partner = household.members.find((m) => m.user.id !== me.id)?.user;

  useEffect(() => textRef.current?.focus(), []);
  useEffect(() => () => {
    if (preview?.startsWith("blob:")) URL.revokeObjectURL(preview);
  }, [preview]);

  async function pick(file: File | undefined) {
    if (!file) return;
    setError(null);
    const small = await shrink(file).catch(() => undefined);
    if (!small) {
      setError(t("moments.photoFailed"));
      return;
    }
    setPhoto(small);
    setPreview(URL.createObjectURL(small));
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const body = { text: text.trim(), visibility, day };
    const current = saved.current;
    const res = await (current
      ? api.PUT("/v1/moments/{momentId}", { params: { path: { momentId: current.id } }, body })
      : api.POST("/v1/households/{householdId}/moments", { params: { path: { householdId: household.id } }, body: { ...body, kind: "moment" } })
    ).catch(() => undefined);
    if (!res?.data) {
      setBusy(false);
      setError(res?.error?.message ?? t("moments.failed"));
      return;
    }
    saved.current = res.data;
    let ok = true;
    if (photo) ok = !!(await uploadPhoto(res.data, photo));
    if (photo === null && res.data.photoUrl) {
      const del = await api.DELETE("/v1/moments/{momentId}/photo", { params: { path: { momentId: res.data.id } } }).catch(() => undefined);
      ok = !!del?.data;
    }
    setBusy(false);
    if (ok) onSaved();
    else setError(t("moments.photoFailed"));
  }

  async function remove() {
    if (!moment || !window.confirm(t("entries.deleteConfirm"))) return;
    setBusy(true);
    const res = await api.DELETE("/v1/moments/{momentId}", { params: { path: { momentId: moment.id } } }).catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onSaved();
    else setError(res?.error?.message ?? t("moments.failed"));
  }

  return (
    <div className="sheet-backdrop" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <form className="sheet" role="dialog" aria-modal="true" aria-label={t(moment ? "moments.edit" : "moments.new")} onSubmit={submit}>
        <div className="sheet-grabber" aria-hidden="true" />
        <div className="sheet-bar">
          <Button variant="plain" onClick={onClose}>
            {t("entries.cancel")}
          </Button>
          <h2 className="headline sheet-title">{t(moment ? "moments.edit" : "moments.new")}</h2>
          <Button type="submit" size="sm" disabled={busy || !text.trim() || !day}>
            {t("entries.save")}
          </Button>
        </div>

        <label className="field">
          <span className="label">{t("moments.text")}</span>
          <textarea ref={textRef} className="input textarea" rows={4} value={text} maxLength={2000} onChange={(e) => setText(e.target.value)} required />
        </label>

        <label className="field">
          <span className="label">{t("moments.day")}</span>
          <input className="input" type="date" value={day} max={today()} onChange={(e) => setDay(e.target.value)} required />
        </label>

        {preview && <img className="moment-photo" src={preview} alt="" />}
        <div className="moment-photo-actions">
          <label className="hl-btn hl-btn-secondary hl-btn-sm">
            {t(preview ? "moments.photoChange" : "moments.photo")}
            <input className="visually-hidden" type="file" accept="image/*" onChange={(e) => pick(e.target.files?.[0])} />
          </label>
          {preview && (
            <Button variant="plain" size="sm" onClick={() => (setPhoto(null), setPreview(undefined))}>
              {t("moments.photoRemove")}
            </Button>
          )}
        </div>

        <fieldset className="sheet-fieldset">
          <legend className="label">{t("entries.who")}</legend>
          <div className="hl-list sheet-choices">
            {(["private", "shared"] as const).map((v) => (
              <label key={v} className="hl-listrow sheet-choice">
                <input type="radio" name="visibility" checked={visibility === v} onChange={() => setVisibility(v)} />
                <span className="hl-listrow-main">
                  <span className="hl-listrow-title">{t(`entries.visibility.${v}`)}</span>
                  <span className="hl-listrow-sub">{t(`entries.visibilityHint.${v}`, { name: partner?.name ?? "" })}</span>
                </span>
                <Icon name={v === "private" ? "lock" : v} size="sm" />
              </label>
            ))}
          </div>
        </fieldset>

        {error && (
          <p className="callout error" role="alert">
            {error}
          </p>
        )}
        {moment && (
          <Button variant="danger" block disabled={busy} onClick={remove}>
            {t("entries.delete")}
          </Button>
        )}
      </form>
    </div>
  );
}
