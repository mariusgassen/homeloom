import { api } from "./api/client";

// Whether this device shares where it is. The browser asks for permission itself; this is the
// person's own switch on top of it, kept per device.
const key = "homeloom.shareLocation";

export function sharing(): boolean {
  try {
    return localStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

function remember(on: boolean) {
  try {
    if (on) localStorage.setItem(key, "1");
    else localStorage.removeItem(key);
  } catch {
    // Private mode: it just isn't remembered.
  }
}

// Where the device is, to about a town; null when there is no geolocation, no permission or no answer.
export function position(): Promise<{ latitude: number; longitude: number } | null> {
  if (!("geolocation" in navigator)) return Promise.resolve(null);
  return new Promise((resolve) =>
    navigator.geolocation.getCurrentPosition(
      (p) => resolve({ latitude: p.coords.latitude, longitude: p.coords.longitude }),
      () => resolve(null),
      { enableHighAccuracy: false, timeout: 8000, maximumAge: 10 * 60 * 1000 },
    ),
  );
}

// Tells the API where the device is. Called from a tap the first time, so the permission prompt shows.
export async function startSharing(): Promise<boolean> {
  const at = await position();
  if (!at) return false;
  const res = await api.PUT("/v1/me/location", { body: at }).catch(() => undefined);
  if (!res || res.error) return false;
  remember(true);
  return true;
}

// Sends the position again, so the weather stays where the person is; a no-op while not sharing.
export async function refreshLocation(): Promise<void> {
  if (!sharing()) return;
  const at = await position();
  if (at) await api.PUT("/v1/me/location", { body: at }).catch(() => undefined);
}

export async function stopSharing(): Promise<void> {
  remember(false);
  await api.DELETE("/v1/me/location").catch(() => undefined);
}
