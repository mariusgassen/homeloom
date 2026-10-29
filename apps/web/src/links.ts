import { useEffect } from "react";
import { useNavigate } from "react-router";

// A tapped notification's link, from the service worker (public/push-sw.js). An open app gets it as
// a message; a closed one finds it kept in the cache, in case iOS started the app at start_url.
const cacheName = "homeloom-link";
const cacheKey = "/__homeloom/link";
// Older than this, a kept link is from some earlier tap and not why the app opened.
const freshFor = 60_000;

type Link = { path: string; at: number };

// takeLink returns the kept link, if it's fresh, and clears it either way.
export async function takeLink(now = Date.now()): Promise<string | null> {
  if (typeof caches === "undefined") return null;
  try {
    const cache = await caches.open(cacheName);
    const res = await cache.match(cacheKey);
    if (!res) return null;
    await cache.delete(cacheKey);
    const link = (await res.json()) as Link;
    return inApp(link.path) && now - link.at < freshFor ? link.path : null;
  } catch {
    return null;
  }
}

// Only paths of this app, never another origin.
function inApp(path: unknown): path is string {
  return typeof path === "string" && path.startsWith("/") && !path.startsWith("//");
}

// useNotificationLinks opens the screen a tapped notification leads to.
export function useNotificationLinks() {
  const navigate = useNavigate();

  useEffect(() => {
    const sw = typeof navigator !== "undefined" ? navigator.serviceWorker : undefined;
    let live = true;
    const go = (path: string) => {
      if (live && path !== location.pathname + location.search + location.hash) void navigate(path);
    };
    const check = () => void takeLink().then((path) => path && go(path));
    const onMessage = (e: MessageEvent) => {
      const data = e.data as { type?: string; path?: unknown } | null;
      if (data?.type !== "homeloom:open" || !inApp(data.path)) return;
      void takeLink();
      go(data.path);
    };
    const onVisible = () => document.visibilityState === "visible" && check();
    check();
    sw?.addEventListener("message", onMessage);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      live = false;
      sw?.removeEventListener("message", onMessage);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [navigate]);
}
