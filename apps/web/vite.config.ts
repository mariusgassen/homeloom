import { createReadStream, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import type { Plugin } from "vite";
import { VitePWA } from "vite-plugin-pwa";
import { defineConfig } from "vitest/config";

// Paths the API serves. In production Caddy routes them to homeloom-api; in development the
// Vite server proxies them, so the session cookie is first-party in both cases.
const apiPaths = ["/auth", "/v1", "/healthz", "/readyz"];

// mediapipe serves MediaPipe's runtime for call effects at /mediapipe/ from Homeloom itself, so
// no camera frame or request goes to a CDN; its models are in public/mediapipe/.
function mediapipe(): Plugin {
  const dir = fileURLToPath(new URL("node_modules/@mediapipe/tasks-vision/wasm/", import.meta.url));
  // The "module" build is only loaded with forVisionTasks(path, true), which nothing here asks for.
  const files = readdirSync(dir).filter((file) => !file.includes("_module_"));
  return {
    name: "homeloom-mediapipe",
    configureServer(server) {
      server.middlewares.use("/mediapipe/", (req, res, next) => {
        const file = req.url?.slice(1).split("?")[0];
        if (!file || !files.includes(file)) return next();
        res.setHeader("Content-Type", file.endsWith(".wasm") ? "application/wasm" : "text/javascript");
        createReadStream(join(dir, file)).pipe(res);
      });
    },
    generateBundle() {
      for (const file of files) this.emitFile({ type: "asset", fileName: `mediapipe/${file}`, source: readFileSync(join(dir, file)) });
    },
  };
}

export default defineConfig({
  plugins: [
    react(),
    mediapipe(),
    VitePWA({
      registerType: "autoUpdate",
      manifest: {
        name: "Homeloom",
        short_name: "Homeloom",
        description: "A shared home for two.",
        lang: "en",
        display: "standalone",
        orientation: "portrait",
        start_url: "/",
        theme_color: "#1f4d47",
        background_color: "#f2f3ee",
        // Copies of brand/; regenerate there, then copy them over.
        icons: [
          { src: "icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
          { src: "icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
          { src: "icon-maskable-192.png", sizes: "192x192", type: "image/png", purpose: "maskable" },
          { src: "icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
        ],
      },
      workbox: {
        // Sign-in and API calls must always reach the server, never the app shell.
        navigateFallbackDenylist: apiPaths.map((p) => new RegExp(`^${p}`)),
        globPatterns: ["**/*.{js,css,html,svg,png,woff2}"],
        // Call effects and video compression load their runtime when first used, not with the app.
        globIgnores: ["mediapipe/**", "assets/effects-*.js", "assets/compress-*.js"],
        // Pictures change their URL when they change, so a kept copy is right until it's gone; they
        // show offline too. The cache is dropped on sign-out (src/cache.ts).
        runtimeCaching: [
          {
            urlPattern: /\/v1\/(users\/[^/]+\/avatar|entries\/[^/]+\/photos\/[^/]+|moments\/[^/]+\/photo|households\/[^/]+\/recipes\/[^/]+\/image)(\?|$)/,
            handler: "CacheFirst",
            options: {
              cacheName: "homeloom-pictures",
              cacheableResponse: { statuses: [200] },
              expiration: { maxEntries: 300, maxAgeSeconds: 30 * 24 * 60 * 60 },
            },
          },
        ],
        // Web Push: showing notifications and opening them (public/push-sw.js).
        importScripts: ["push-sw.js"],
      },
    }),
  ],
  server: {
    proxy: {
      ...Object.fromEntries(apiPaths.map((p) => [p, "http://localhost:8080"])),
      // Call signalling, to homeloom-livekit from docker-compose.dev.yml.
      "/rtc": { target: "http://localhost:7880", ws: true },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["src/test-setup.ts"],
  },
});
