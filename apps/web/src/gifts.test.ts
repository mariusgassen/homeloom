import { describe, expect, it } from "vitest";
import { myBirthdayValue } from "./gifts";

describe("myBirthdayValue", () => {
  const saved = { month: 5, day: 4 };

  it("keeps a year before 2000 while it names the saved day", () => {
    expect(myBirthdayValue(saved, "1990-05-04")).toBe("1990-05-04");
  });

  it("falls back to the saved day in a leap year", () => {
    expect(myBirthdayValue(saved, "")).toBe("2000-05-04");
    expect(myBirthdayValue(saved, "1990-06-01")).toBe("2000-05-04");
  });

  it("shows what was picked before anything is saved", () => {
    expect(myBirthdayValue(undefined, "1985-01-02")).toBe("1985-01-02");
  });
});
