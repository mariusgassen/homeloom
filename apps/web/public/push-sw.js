// Loaded into the generated service worker (vite.config.ts, workbox.importScripts). The API sends
// { title, body, url, tag }, already written in the device's language.
self.addEventListener("push", (event) => {
  let msg = {};
  try {
    msg = event.data ? event.data.json() : {};
  } catch {
    // A push the app can't read still has to show something, or iOS stops delivering them.
  }
  event.waitUntil(
    self.registration.showNotification(msg.title || "Homeloom", {
      body: msg.body,
      tag: msg.tag,
      icon: "/icon-192.png",
      data: { url: msg.url || "/" },
    }),
  );
});

// Where the last tapped notification leads; src/links.ts reads and clears it.
const LINK_CACHE = "homeloom-link";
const LINK_KEY = "/__homeloom/link";

// Tapping a notification opens its page. An open app window is brought forward and told where to
// go (src/links.ts): WindowClient.navigate fails on iOS and would reload the app, ending a call. The
// link is also kept for a moment, because iOS may start a closed app at start_url instead.
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = new URL(event.notification.data?.url || "/", self.location.origin);
  const path = url.origin === self.location.origin ? url.pathname + url.search + url.hash : "/";
  event.waitUntil(
    caches
      .open(LINK_CACHE)
      .then((c) => c.put(LINK_KEY, new Response(JSON.stringify({ path, at: Date.now() }))))
      .catch(() => undefined)
      .then(() => self.clients.matchAll({ type: "window", includeUncontrolled: true }))
      .then((windows) => {
        const open = windows.find((w) => "focus" in w);
        if (!open) return self.clients.openWindow(path);
        open.postMessage({ type: "homeloom:open", path });
        return open.focus().catch(() => undefined);
      }),
  );
});

// A push service that rotates the subscription hands over a new one; tell the API about it.
self.addEventListener("pushsubscriptionchange", (event) => {
  const options = event.oldSubscription?.options;
  if (!options) return;
  event.waitUntil(
    self.registration.pushManager.subscribe(options).then((sub) =>
      fetch("/v1/push/subscriptions", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(sub.toJSON()),
      }),
    ),
  );
});
