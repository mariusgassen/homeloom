import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import de from "./locales/de.json";
import en from "./locales/en.json";

export const languages = ["en", "de"] as const;

// German or English from the browser's preferences; English is the fallback, as in the API.
export function detect(preferred: readonly string[]): (typeof languages)[number] {
  for (const tag of preferred) {
    const base = tag.toLowerCase().split("-")[0];
    const match = languages.find((l) => l === base);
    if (match) return match;
  }
  return "en";
}

i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, de: { translation: de } },
  lng: detect(navigator.languages),
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});
document.documentElement.lang = i18n.language;

export default i18n;
