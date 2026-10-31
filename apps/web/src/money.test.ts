import { expect, test } from "vitest";
import { centsText, nextBill, parseCents } from "./money";

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

test("nextBill picks the open bill due soonest within two weeks", () => {
  const now = new Date("2026-10-30T08:00:00Z");
  const bill = { cents: 90000, category: "home" as const };
  const entry = (id: string, dueAt: string, extra = {}) => ({ id, kind: "reminder" as const, authorId: "a", visibility: "shared" as const, repeat: "monthly" as const, sealed: false, allDay: false, createdAt: "", updatedAt: "", text: id, dueAt, bill, ...extra });
  expect(
    nextBill([entry("later", "2026-11-10T09:00:00Z"), entry("rent", "2026-11-01T09:00:00Z"), entry("far", "2026-12-24T09:00:00Z"), entry("done", "2026-10-31T09:00:00Z", { doneAt: "2026-10-29T09:00:00Z" }), entry("plain", "2026-10-30T09:00:00Z", { bill: undefined })], now)?.id,
  ).toBe("rent");
  expect(nextBill([entry("far", "2026-12-24T09:00:00Z")], now)).toBeUndefined();
});
