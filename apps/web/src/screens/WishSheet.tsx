import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type HouseholdDetail, type Wish } from "../api/client";
import { centsText, parseCents } from "../money";
import { shrink } from "../moments";
import { Button } from "../ui/Button";
import { FormGroup } from "../ui/Form";
import { Icon } from "../ui/Icon";
import { Sheet } from "../ui/Sheet";

// Adding or changing a wish: what it is, a note, where to find it, what it costs and a photo.
export function WishSheet({
  household,
  wish,
  onClose,
  onSaved,
}: {
  household: HouseholdDetail;
  wish?: Wish;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [title, setTitle] = useState(wish?.title ?? "");
  const [note, setNote] = useState(wish?.note ?? "");
  const [link, setLink] = useState(wish?.url ?? "");
  const [price, setPrice] = useState(
    wish?.priceCents ? centsText(wish.priceCents, i18n.language) : "",
  );
  // undefined keeps the photo as it is, null removes it, a Blob replaces it.
  const [photo, setPhoto] = useState<Blob | null | undefined>(undefined);
  const [preview, setPreview] = useState<string | undefined>(wish?.photoUrl);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A wish saved before its photo failed is changed, not added again, on the next try.
  const saved = useRef<Wish | undefined>(wish);
  const titleRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!wish) titleRef.current?.focus();
  }, [wish]);
  useEffect(
    () => () => {
      if (preview?.startsWith("blob:")) URL.revokeObjectURL(preview);
    },
    [preview],
  );

  async function pick(file: File | undefined) {
    if (!file) return;
    setError(null);
    const small = await shrink(file).catch(() => undefined);
    if (!small) return setError(t("wishes.photoFailed"));
    setPhoto(small);
    setPreview(URL.createObjectURL(small));
  }

  const cents = price.trim() ? parseCents(price) : undefined;
  const ready = title.trim() !== "" && (cents === undefined || cents > 0);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!ready) return;
    setBusy(true);
    setError(null);
    const body = {
      title: title.trim(),
      note: note.trim() || undefined,
      url: link.trim() || undefined,
      priceCents: cents,
    };
    const current = saved.current;
    const res = await (
      current
        ? api.PUT("/v1/households/{householdId}/wishes/{wishId}", {
            params: { path: { householdId: household.id, wishId: current.id } },
            body,
          })
        : api.POST("/v1/households/{householdId}/wishes", {
            params: { path: { householdId: household.id } },
            body,
          })
    ).catch(() => undefined);
    if (!res?.data) {
      setBusy(false);
      setError(res?.error?.message ?? t("wishes.failed"));
      return;
    }
    saved.current = res.data;
    const path = {
      params: { path: { householdId: household.id, wishId: res.data.id } },
    };
    let result: { error?: { message: string } } | undefined = {};
    if (photo) {
      result = await api
        .PUT("/v1/households/{householdId}/wishes/{wishId}/photo", {
          ...path,
          // The contract types a binary body as a string; the photo goes as it is.
          body: photo as unknown as string,
          bodySerializer: (b) => b,
          headers: { "Content-Type": "image/jpeg" },
        })
        .catch(() => undefined);
    } else if (photo === null && res.data.photoUrl) {
      result = await api
        .DELETE("/v1/households/{householdId}/wishes/{wishId}/photo", path)
        .catch(() => undefined);
    }
    setBusy(false);
    if (result && !result.error) onSaved();
    else setError(result?.error?.message ?? t("wishes.photoFailed"));
  }

  async function remove() {
    if (!wish || !window.confirm(t("wishes.deleteConfirm"))) return;
    setBusy(true);
    const res = await api
      .DELETE("/v1/households/{householdId}/wishes/{wishId}", {
        params: { path: { householdId: household.id, wishId: wish.id } },
      })
      .catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onSaved();
    else setError(res?.error?.message ?? t("wishes.failed"));
  }

  return (
    <Sheet
      title={t(wish ? "wishes.edit" : "wishes.new")}
      onClose={onClose}
      onSubmit={submit}
      leading={
        <Button variant="plain" onClick={onClose}>
          {t("entries.cancel")}
        </Button>
      }
      trailing={
        <Button type="submit" size="sm" disabled={busy || !ready}>
          {t("entries.save")}
        </Button>
      }
    >
      <label className="field">
        <span className="label">{t("wishes.what")}</span>
        <input
          ref={titleRef}
          className="input"
          value={title}
          maxLength={100}
          onChange={(e) => setTitle(e.target.value)}
          placeholder={t("wishes.placeholder")}
          enterKeyHint="next"
          required
        />
      </label>

      <FormGroup>
        <label className="field">
          <span className="label">{t("wishes.price")}</span>
          <input
            className="input"
            value={price}
            inputMode="decimal"
            onChange={(e) => setPrice(e.target.value)}
            placeholder="0,00"
            aria-invalid={cents !== undefined && !(cents > 0)}
          />
        </label>
        <label className="field">
          <span className="label">{t("wishes.link")}</span>
          <input
            className="input"
            type="url"
            value={link}
            maxLength={2000}
            onChange={(e) => setLink(e.target.value)}
            placeholder="https://"
            autoCapitalize="none"
            autoCorrect="off"
          />
        </label>
        <label className="field">
          <span className="label">{t("wishes.note")}</span>
          <textarea
            className="input"
            value={note}
            maxLength={500}
            rows={3}
            onChange={(e) => setNote(e.target.value)}
          />
        </label>
      </FormGroup>

      {preview && (
        <img className="moment-photo" src={preview} alt={t("wishes.photo")} />
      )}
      <div className="moment-photo-actions">
        <label className="hl-btn hl-btn-secondary hl-btn-sm">
          <Icon name="camera" size="sm" />
          {t(preview ? "wishes.photoChange" : "wishes.photoAdd")}
          <input
            className="visually-hidden"
            type="file"
            accept="image/*"
            onChange={(e) => pick(e.target.files?.[0])}
          />
        </label>
        {preview && (
          <Button
            variant="plain"
            size="sm"
            onClick={() => (setPhoto(null), setPreview(undefined))}
          >
            {t("wishes.photoRemove")}
          </Button>
        )}
      </div>

      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {wish && (
        <Button variant="danger" block disabled={busy} onClick={remove}>
          {t("wishes.delete")}
        </Button>
      )}
    </Sheet>
  );
}
