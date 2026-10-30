import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type HouseholdDetail, type Moment, type User, type Visibility } from "../api/client";
import { maxVideoMB, shrink, today, uploadPhoto, uploadVideo } from "../moments";
import { Button } from "../ui/Button";
import { DateRow, FormGroup, Segmented } from "../ui/Form";
import { Sheet } from "../ui/Sheet";

// Keep a moment: what happened, the day, a photo and a video if there are any, and who sees it. With a moment it
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
  // The same for the video.
  const [video, setVideo] = useState<File | null | undefined>(undefined);
  const [videoPreview, setVideoPreview] = useState<string | undefined>(moment?.videoUrl);
  // How much of the video has gone, while it is sent.
  const [progress, setProgress] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A moment saved before its photo failed is changed, not made again, on the next try.
  const saved = useRef<Moment | undefined>(moment);
  const textRef = useRef<HTMLTextAreaElement>(null);
  const partner = household.members.find((m) => m.user.id !== me.id)?.user;

  useEffect(() => {
    if (!moment) textRef.current?.focus();
  }, [moment]);
  useEffect(() => () => {
    if (preview?.startsWith("blob:")) URL.revokeObjectURL(preview);
  }, [preview]);
  useEffect(() => () => {
    if (videoPreview?.startsWith("blob:")) URL.revokeObjectURL(videoPreview);
  }, [videoPreview]);

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

  function pickVideo(file: File | undefined) {
    if (!file) return;
    if (file.size > maxVideoMB << 20) {
      setError(t("moments.videoTooBig", { max: maxVideoMB }));
      return;
    }
    setError(null);
    setVideo(file);
    setVideoPreview(URL.createObjectURL(file));
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
    let failed: string | null = null;
    if (photo && !(await uploadPhoto(res.data, photo))) failed = t("moments.photoFailed");
    if (photo === null && res.data.photoUrl) {
      const del = await api.DELETE("/v1/moments/{momentId}/photo", { params: { path: { momentId: res.data.id } } }).catch(() => undefined);
      if (!del?.data) failed = t("moments.photoFailed");
    }
    if (video) {
      setProgress(0);
      const sent = await uploadVideo(res.data, video, setProgress);
      setProgress(null);
      // Sent once is enough; a retry only sends what failed.
      if (sent.moment) setVideo(undefined);
      else failed = sent.error ?? t("moments.videoFailed");
    }
    if (video === null && res.data.videoUrl) {
      const del = await api.DELETE("/v1/moments/{momentId}/video", { params: { path: { momentId: res.data.id } } }).catch(() => undefined);
      if (!del?.data) failed = t("moments.videoFailed");
    }
    setBusy(false);
    if (failed) setError(failed);
    else onSaved();
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
    <Sheet
      title={t(moment ? "moments.edit" : "moments.new")}
      onClose={onClose}
      onSubmit={submit}
      leading={
        <Button variant="plain" onClick={onClose}>
          {t("entries.cancel")}
        </Button>
      }
      trailing={
        <Button type="submit" size="sm" disabled={busy || !text.trim() || !day}>
          {t("entries.save")}
        </Button>
      }
    >
      <label className="field">
        <span className="label">{t("moments.text")}</span>
        <textarea ref={textRef} className="input textarea" rows={4} value={text} maxLength={2000} onChange={(e) => setText(e.target.value)} required />
      </label>

      <FormGroup>
        <DateRow label={t("moments.day")} type="date" value={day} max={today()} onChange={setDay} placeholder={t("entries.pick")} required />
      </FormGroup>

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

      {videoPreview && <video className="moment-photo" src={videoPreview} controls playsInline muted preload="metadata" />}
      <div className="moment-photo-actions">
        <label className="hl-btn hl-btn-secondary hl-btn-sm">
          {t(videoPreview ? "moments.videoChange" : "moments.video")}
          <input className="visually-hidden" type="file" accept="video/mp4,video/quicktime" onChange={(e) => pickVideo(e.target.files?.[0])} />
        </label>
        {videoPreview && (
          <Button variant="plain" size="sm" onClick={() => (setVideo(null), setVideoPreview(undefined))}>
            {t("moments.videoRemove")}
          </Button>
        )}
      </div>
      {progress !== null && (
        <p className="caption muted" role="status">
          {t("moments.videoSending", { percent: Math.round(progress * 100) })}
        </p>
      )}

      <div className="field">
        <span className="label">{t("entries.who")}</span>
        <Segmented
          label={t("entries.who")}
          value={visibility}
          options={(["private", "shared"] as const).map((v) => ({ value: v, label: t(`entries.visibility.${v}`), icon: v === "private" ? "lock" : v }))}
          onChange={setVisibility}
        />
        <p className="caption muted sheet-hint">{t(`entries.visibilityHint.${visibility}`, { name: partner?.name ?? "" })}</p>
      </div>

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
    </Sheet>
  );
}
