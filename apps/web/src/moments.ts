import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { api, type Moment, type MomentMonth, type Resurfaced } from "./api/client";
import { cached } from "./cache";
import { useRefresh } from "./refresh";

// The timeline, a month at a time: the newest month first (or the month asked for), and older
// ones as the person asks for them.
export function useTimeline(householdId: string, month?: string) {
  const [months, setMonths] = useState<MomentMonth[] | null>(() => {
    const kept = cached<MomentMonth>(`/v1/households/${householdId}/moments`, { month });
    return kept ? [kept] : null;
  });
  const [failed, setFailed] = useState(false);
  const [generation, setGeneration] = useState(0);
  const { generation: refreshed, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/moments", { params: { path: { householdId }, query: month ? { month } : {} } })
      .then((res) => {
        if (!current) return;
        if (res.data) setMonths([res.data]);
        setFailed(!res.data);
      })
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, month, generation, refreshed, track]);

  const earlier = months?.at(-1)?.earlier;
  const loadEarlier = useCallback(async () => {
    if (!earlier) return;
    const res = await api.GET("/v1/households/{householdId}/moments", { params: { path: { householdId }, query: { month: earlier } } }).catch(() => undefined);
    if (res?.data) {
      const next = res.data;
      setMonths((ms) => [...(ms ?? []), next]);
    } else setFailed(true);
  }, [householdId, earlier]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  return { months, failed, earlier, loadEarlier, reload };
}

// What comes back today: this day in earlier years and a gratitude line from a while ago.
export function useResurfaced(householdId: string, generation: number) {
  const [resurfaced, setResurfaced] = useState<Resurfaced | null>(() => cached<Resurfaced>(`/v1/households/${householdId}/moments/resurfaced`) ?? null);
  const { generation: refreshed, track } = useRefresh();
  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/moments/resurfaced", { params: { path: { householdId } } })
      .then((res) => current && setResurfaced(res.data ?? null), () => undefined);
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed, track]);
  return resurfaced;
}

// Photos are made at most this many pixels on their long side before they are sent.
const photoSize = 1600;

// shrink turns a picked photo into a JPEG small enough to send; createImageBitmap reads HEIC on
// iOS and turns the picture upright.
export async function shrink(file: Blob): Promise<Blob> {
  const bitmap = await createImageBitmap(file);
  const scale = Math.min(1, photoSize / Math.max(bitmap.width, bitmap.height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.round(bitmap.width * scale);
  canvas.height = Math.round(bitmap.height * scale);
  canvas.getContext("2d")?.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
  bitmap.close();
  return new Promise((resolve, reject) => canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("photo"))), "image/jpeg", 0.82));
}

export async function uploadPhoto(moment: Moment, photo: Blob) {
  const res = await api
    .PUT("/v1/moments/{momentId}/photo", {
      params: { path: { momentId: moment.id } },
      // The contract types a binary body as a string; the photo goes as it is.
      body: photo as unknown as string,
      bodySerializer: (b) => b,
      headers: { "Content-Type": "image/jpeg" },
    })
    .catch(() => undefined);
  return res?.data;
}

// The largest video the API takes, in MB; the app compresses videos first where it can.
export const maxVideoMB = 200;

// uploadVideo sends a picked video as it is, reporting how much has gone. It uses XHR, because
// fetch can't tell how far an upload is. It resolves to the moment, or to the API's message.
export function uploadVideo(moment: Moment, video: Blob, onProgress: (share: number) => void) {
  return new Promise<{ moment?: Moment; error?: string }>((resolve) => {
    const xhr = new XMLHttpRequest();
    xhr.open("PUT", `/v1/moments/${moment.id}/video`);
    const mov = video instanceof File && /\.mov$/i.test(video.name);
    xhr.setRequestHeader("Content-Type", video.type || (mov ? "video/quicktime" : "video/mp4"));
    xhr.responseType = "json";
    xhr.upload.onprogress = (e) => e.lengthComputable && onProgress(e.loaded / e.total);
    xhr.onload = () => {
      const body = xhr.response as (Moment & { message?: string }) | null;
      resolve(xhr.status === 200 && body ? { moment: body } : { error: body?.message });
    };
    xhr.onerror = () => resolve({});
    xhr.send(video);
  });
}

// Whether videos on the timeline play by themselves, muted, while they are in view. On unless
// turned off on this device; kept here as well, in case the browser keeps nothing.
const autoplayKey = "homeloom.autoplayVideos";
let autoplay = (() => {
  try {
    return localStorage.getItem(autoplayKey) !== "0";
  } catch {
    return true;
  }
})();
const autoplayListeners = new Set<() => void>();

export function useAutoplay() {
  const on = useSyncExternalStore(
    (listener) => {
      autoplayListeners.add(listener);
      return () => autoplayListeners.delete(listener);
    },
    () => autoplay,
  );
  const set = useCallback((v: boolean) => {
    autoplay = v;
    try {
      if (v) localStorage.removeItem(autoplayKey);
      else localStorage.setItem(autoplayKey, "0");
    } catch {
      // Not remembered, but still switched.
    }
    autoplayListeners.forEach((l) => l());
  }, []);
  return [on, set] as const;
}

// A moment's day as a local date, for grouping and formatting; the API sends a bare date.
export function dayOf(m: Moment) {
  const [y, mo, d] = m.day.split("-").map(Number);
  return new Date(y ?? 0, (mo ?? 1) - 1, d ?? 1);
}

// The local date as the API writes one.
export function today() {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
