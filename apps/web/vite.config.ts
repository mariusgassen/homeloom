import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";
import { defineConfig } from "vitest/config";

// Paths the API serves. In production Caddy routes them to homeloom-api; in development the
// Vite server proxies them, so the session cookie is first-party in both cases.
const apiPaths = ["/auth", "/v1", "/healthz", "/readyz"];

export default defineConfig({
  plugins: [
    react(),
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
