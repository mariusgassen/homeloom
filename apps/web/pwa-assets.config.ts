import { defineConfig, minimal2023Preset } from "@vite-pwa/assets-generator/config";

export default defineConfig({
  headLinkOptions: { preset: "2023" },
  preset: {
    ...minimal2023Preset,
    // iOS shows the home-screen icon on a solid tile; match the brand colour instead of white.
    apple: { ...minimal2023Preset.apple, resizeOptions: { background: "#1f4d47" } },
    maskable: { ...minimal2023Preset.maskable, resizeOptions: { background: "#1f4d47" } },
  },
  images: ["public/icon.svg"],
});
