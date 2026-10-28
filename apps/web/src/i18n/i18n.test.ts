import { expect, test } from "vitest";
import { detect } from ".";
import de from "./locales/de.json";
import en from "./locales/en.json";

function keys(o: object, prefix = ""): string[] {
  return Object.entries(o).flatMap(([k, v]) => (typeof v === "object" ? keys(v, `${prefix}${k}.`) : [`${prefix}${k}`]));
}

test("every key exists in German and English", () => {
  expect(keys(de).sort()).toEqual(keys(en).sort());
});

test("detect picks German or English, falling back to English", () => {
  expect(detect(["de-DE", "en"])).toBe("de");
  expect(detect(["fr-FR", "en-GB"])).toBe("en");
  expect(detect(["fr"])).toBe("en");
  expect(detect([])).toBe("en");
});
