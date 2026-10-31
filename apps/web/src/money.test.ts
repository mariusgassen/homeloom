import { expect, test } from "vitest";
import { centsText, parseCents } from "./money";

test("parseCents reads amounts as people type them", () => {
  expect(parseCents("12,50")).toBe(1250);
  expect(parseCents("12.5")).toBe(1250);
  expect(parseCents("€ 7")).toBe(700);
  expect(parseCents("0,01")).toBe(1);
  expect(parseCents("")).toBeNaN();
  expect(parseCents("1.234,50")).toBeNaN();
  expect(parseCents("12.345")).toBeNaN();
  expect(parseCents("-3")).toBeNaN();
});

test("centsText writes the decimal mark of the language", () => {
  expect(centsText(1250, "de")).toBe("12,50");
  expect(centsText(1250, "en")).toBe("12.50");
});
