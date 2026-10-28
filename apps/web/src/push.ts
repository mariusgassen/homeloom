import { api } from "./api/client";

// Web Push on iOS only works in the app added to the home screen (iOS 16.4 and later); in Safari
// itself PushManager doesn't exist, so the app explains how to install instead.
export type PushSupport = "ok" | "needsInstall" | "unsupported";

export function pushSupport(): PushSupport {
  if ("serviceWorker" in navigator && "PushManager" in window && "Notification" in window) return "ok";
  const ios = /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  const standalone = window.matchMedia?.("(display-mode: standalone)").matches || (navigator as { standalone?: boolean }).standalone === true;
  return ios && !standalone ? "needsInstall" : "unsupported";
}

// The server's VAPID key, or null when the server has push switched off.
export async function pushKey(): Promise<string | null> {
  const res = await api.GET("/v1/push").catch(() => undefined);
  return res?.data?.publicKey ?? null;
}

export async function currentSubscription(): Promise<PushSubscription | null> {
  const reg = await navigator.serviceWorker.ready;
  return reg.pushManager.getSubscription();
}

// subscribe must run from a tap: iOS only shows the permission prompt for a user gesture, so the
// prompt comes first, before anything else is awaited. lang is what notifications are written in.
export async function subscribe(publicKey: string, lang: string): Promise<boolean> {
  if ((await Notification.requestPermission()) !== "granted") return false;
  const reg = await navigator.serviceWorker.ready;
  const sub = await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: fromBase64Url(publicKey) });
  const { endpoint, keys } = sub.toJSON();
  const res = await api.POST("/v1/push/subscriptions", {
    body: { endpoint: endpoint ?? "", keys: { p256dh: keys?.p256dh ?? "", auth: keys?.auth ?? "" } },
    headers: { "Accept-Language": lang },
  });
  if (res.response.ok) return true;
  await sub.unsubscribe();
  return false;
}

export async function unsubscribe(sub: PushSubscription): Promise<void> {
  await api.DELETE("/v1/push/subscriptions", { params: { query: { endpoint: sub.endpoint } } });
  await sub.unsubscribe();
}

function fromBase64Url(s: string): Uint8Array<ArrayBuffer> {
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}
