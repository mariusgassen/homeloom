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

// Tapping a notification opens its page, in the app window if one is open.
self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = new URL(event.notification.data?.url || "/", self.location.origin).href;
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((windows) => {
      const open = windows.find((w) => "focus" in w);
      if (open) return open.navigate(url).then((w) => (w || open).focus());
      return self.clients.openWindow(url);
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
