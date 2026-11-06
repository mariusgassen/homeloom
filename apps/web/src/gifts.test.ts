import { describe, expect, it } from "vitest";
import { birthdayValue, parseBirthday, turns } from "./gifts";

describe("birthday", () => {
  it("keeps a year before 2000 in the date input", () => {
    expect(parseBirthday("1990-05-04")).toEqual({ year: 1990, month: 5, day: 4 });
    expect(birthdayValue({ month: 5, day: 4, year: 1990 })).toBe("1990-05-04");
  });

  it("falls back to a leap year without a year of birth", () => {
    expect(birthdayValue({ month: 2, day: 29 })).toBe("2000-02-29");
  });

  it("works out the age they turn", () => {
    expect(turns({ birthday: { year: 1990 }, next: "2026-05-04" })).toBe(36);
    expect(turns({ birthday: {}, next: "2026-05-04" })).toBeUndefined();
  });
});
