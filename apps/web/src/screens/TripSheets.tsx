import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type Trip, type TripFields, type TripOption, type TripOptionFields } from "../api/client";
import { shrink } from "../moments";
import { uploadTripPhoto } from "../trips";
import { Button } from "../ui/Button";
import { DateRow, FormGroup } from "../ui/Form";
import { Icon } from "../ui/Icon";
import { Sheet } from "../ui/Sheet";

const bar = (onClose: () => void, busy: boolean, ready: boolean, cancel: string, save: string) => ({
  leading: (
    <Button variant="plain" onClick={onClose}>
      {cancel}
    </Button>
  ),
  trailing: (
    <Button type="submit" size="sm" disabled={busy || !ready}>
      {save}
    </Button>
  ),
});

// Starting or changing a trip: its name, where to, when and a few notes.
export function TripSheet({ householdId, trip, onClose, onSaved, onDeleted }: {
  householdId: string;
  trip?: Trip;
  onClose: () => void;
  onSaved: (trip: Trip) => void;
  onDeleted?: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(trip?.name ?? "");
  const [destination, setDestination] = useState(trip?.destination ?? "");
  const [startOn, setStartOn] = useState(trip?.startOn ?? "");
  const [endOn, setEndOn] = useState(trip?.endOn ?? "");
  const [notes, setNotes] = useState(trip?.notes ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const nameRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!trip) nameRef.current?.focus();
  }, [trip]);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const body: TripFields = {
      name: name.trim(),
      destination: destination.trim() || undefined,
      startOn: startOn || undefined,
      endOn: endOn || undefined,
      notes: notes.trim() || undefined,
      chosenOptionId: trip?.chosenOptionId,
    };
    const path = { householdId };
    const res = await (trip
      ? api.PUT("/v1/households/{householdId}/trips/{tripId}", { params: { path: { ...path, tripId: trip.id } }, body })
      : api.POST("/v1/households/{householdId}/trips", { params: { path }, body })
    ).catch(() => undefined);
    setBusy(false);
    if (res?.data) onSaved(res.data);
    else setError(res?.error?.message ?? t("trips.failed"));
  }

  async function remove() {
    if (!trip || !window.confirm(t("trips.deleteConfirm"))) return;
    setBusy(true);
    const res = await api.DELETE("/v1/households/{householdId}/trips/{tripId}", { params: { path: { householdId, tripId: trip.id } } }).catch(() => undefined);
    setBusy(false);
    if (res?.response.ok) onDeleted?.();
    else setError(res?.error?.message ?? t("trips.failed"));
  }

  return (
    <Sheet title={t(trip ? "trips.edit" : "trips.new")} onClose={onClose} onSubmit={submit} {...bar(onClose, busy, !!name.trim(), t("entries.cancel"), t("entries.save"))}>
      <label className="field">
        <span className="label">{t("trips.name")}</span>
        <input ref={nameRef} className="input" value={name} maxLength={80} onChange={(e) => setName(e.target.value)} placeholder={t("trips.namePlaceholder")} required />
      </label>
      <label className="field">
        <span className="label">{t("trips.destination")}</span>
        <input className="input" value={destination} maxLength={80} onChange={(e) => setDestination(e.target.value)} placeholder={t("trips.destinationPlaceholder")} />
      </label>
      <FormGroup>
        <DateRow label={t("trips.from")} type="date" value={startOn} onChange={(v) => (setStartOn(v), endOn && v > endOn && setEndOn(v))} placeholder={t("entries.pick")} />
        <DateRow label={t("trips.to")} type="date" value={endOn} min={startOn || undefined} onChange={setEndOn} placeholder={t("entries.pick")} />
      </FormGroup>
      <label className="field">
        <span className="label">{t("trips.notes")}</span>
        <textarea className="input textarea" rows={3} value={notes} maxLength={1000} onChange={(e) => setNotes(e.target.value)} />
      </label>
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {trip && (
        <Button variant="danger" block disabled={busy} onClick={remove}>
          {t("trips.delete")}
        </Button>
      )}
    </Sheet>
  );
}

type Added = { blob: Blob; preview: string };

// Adding or changing a place to weigh up: a name, a link, notes and photos. Photos picked here go up
// once the place is saved; the ones already there can be taken off.
export function OptionSheet({ householdId, trip, option, onClose, onSaved }: {
  householdId: string;
  trip: Trip;
  option?: TripOption;
  onClose: () => void;
  onSaved: (trip: Trip) => void;
}) {
  const { t } = useTranslation();
  const [title, setTitle] = useState(option?.title ?? "");
  const [url, setUrl] = useState(option?.url ?? "");
  const [notes, setNotes] = useState(option?.notes ?? "");
  const [added, setAdded] = useState<Added[]>([]);
  const [removed, setRemoved] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A place saved before its photos went up is changed, not added again, on the next try.
  const saved = useRef<{ trip: Trip; optionId: string } | undefined>(option ? { trip, optionId: option.id } : undefined);
  const titleRef = useRef<HTMLInputElement>(null);
  const previews = useRef(added);
  useEffect(() => {
    previews.current = added;
  });
  useEffect(() => {
    if (!option) titleRef.current?.focus();
  }, [option]);
  useEffect(() => () => previews.current.forEach((a) => URL.revokeObjectURL(a.preview)), []);

  const kept = (option?.photos ?? []).filter((p) => !removed.includes(p.id));
  const room = 12 - kept.length - added.length;

  async function pick(files: FileList | null) {
    if (!files) return;
    setError(null);
    const picked: Added[] = [];
    for (const file of Array.from(files).slice(0, Math.max(0, room))) {
      const blob = await shrink(file).catch(() => undefined);
      if (blob) picked.push({ blob, preview: URL.createObjectURL(blob) });
      else setError(t("trips.photoFailed"));
    }
    setAdded((a) => [...a, ...picked]);
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const body: TripOptionFields = { title: title.trim(), url: url.trim() || undefined, notes: notes.trim() || undefined };
    const path = { householdId, tripId: trip.id };
    let current = saved.current;
    if (!current) {
      const res = await api.POST("/v1/households/{householdId}/trips/{tripId}/options", { params: { path }, body }).catch(() => undefined);
      const created = res?.data?.options.at(-1);
      if (!res?.data || !created) {
        setBusy(false);
        return setError(res?.error?.message ?? t("trips.failed"));
      }
      current = saved.current = { trip: res.data, optionId: created.id };
    } else {
      const res = await api
        .PUT("/v1/households/{householdId}/trips/{tripId}/options/{optionId}", { params: { path: { ...path, optionId: current.optionId } }, body })
        .catch(() => undefined);
      if (!res?.data) {
        setBusy(false);
        return setError(res?.error?.message ?? t("trips.failed"));
      }
      current.trip = res.data;
    }
    let failed: string | undefined;
    for (const id of removed) {
      const res = await api
        .DELETE("/v1/households/{householdId}/trips/{tripId}/photos/{photoId}", { params: { path: { ...path, photoId: id } } })
        .catch(() => undefined);
      if (res?.data) current.trip = res.data;
      else failed ??= res?.error?.message ?? t("trips.photoFailed");
    }
    setRemoved((r) => r.filter((id) => current.trip.options.some((o) => o.photos.some((p) => p.id === id))));
    for (const a of [...added]) {
      const res = await uploadTripPhoto(householdId, trip.id, current.optionId, a.blob);
      if (res.trip) {
        current.trip = res.trip;
        URL.revokeObjectURL(a.preview);
        setAdded((list) => list.filter((x) => x !== a));
      } else failed ??= res.error ?? t("trips.photoFailed");
    }
    setBusy(false);
    if (failed) setError(t("trips.savedPhotoFailed", { reason: failed }));
    else onSaved(current.trip);
  }

  async function remove() {
    if (!option || !window.confirm(t("trips.optionDeleteConfirm"))) return;
    setBusy(true);
    const res = await api
      .DELETE("/v1/households/{householdId}/trips/{tripId}/options/{optionId}", { params: { path: { householdId, tripId: trip.id, optionId: option.id } } })
      .catch(() => undefined);
    setBusy(false);
    if (res?.data) onSaved(res.data);
    else setError(res?.error?.message ?? t("trips.failed"));
  }

  return (
    <Sheet title={t(option ? "trips.optionEdit" : "trips.optionNew")} onClose={onClose} onSubmit={submit} {...bar(onClose, busy, !!title.trim(), t("entries.cancel"), t("entries.save"))}>
      <label className="field">
        <span className="label">{t("trips.optionTitle")}</span>
        <input ref={titleRef} className="input" value={title} maxLength={100} onChange={(e) => setTitle(e.target.value)} placeholder={t("trips.optionPlaceholder")} required />
      </label>
      <label className="field">
        <span className="label">{t("trips.link")}</span>
        <input className="input" type="url" inputMode="url" value={url} maxLength={500} onChange={(e) => setUrl(e.target.value)} placeholder="https://" />
      </label>
      <label className="field">
        <span className="label">{t("trips.notes")}</span>
        <textarea className="input textarea" rows={3} value={notes} maxLength={1000} onChange={(e) => setNotes(e.target.value)} />
      </label>

      {(kept.length > 0 || added.length > 0) && (
        <ul className="trip-thumbs" aria-label={t("trips.photos")}>
          {kept.map((p) => (
            <li key={p.id}>
              <img src={p.url} alt="" />
              <button type="button" className="trip-thumb-x" aria-label={t("trips.photoRemove")} onClick={() => setRemoved((r) => [...r, p.id])}>
                <Icon name="close" size="sm" />
              </button>
            </li>
          ))}
          {added.map((a) => (
            <li key={a.preview}>
              <img src={a.preview} alt="" />
              <button type="button" className="trip-thumb-x" aria-label={t("trips.photoRemove")} onClick={() => (URL.revokeObjectURL(a.preview), setAdded((list) => list.filter((x) => x !== a)))}>
                <Icon name="close" size="sm" />
              </button>
            </li>
          ))}
        </ul>
      )}
      {room > 0 && (
        <div className="moment-photo-actions">
          <label className="hl-btn hl-btn-secondary hl-btn-sm">
            <Icon name="camera" size="sm" />
            {t("trips.photoAdd")}
            <input className="visually-hidden" type="file" accept="image/*" multiple onChange={(e) => (void pick(e.target.files), (e.target.value = ""))} />
          </label>
        </div>
      )}

      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {option && (
        <Button variant="danger" block disabled={busy} onClick={remove}>
          {t("trips.optionDelete")}
        </Button>
      )}
    </Sheet>
  );
}
