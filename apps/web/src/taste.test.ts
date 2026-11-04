import { describe, expect, it } from "vitest";
import type { MealRating } from "./api/client";
import { ratingsOf } from "./taste";

const r = (slug: string, userId: string, rating: MealRating["rating"]): MealRating => ({ slug, name: slug, userId, rating, ratedAt: "2026-11-04T10:00:00Z" });

describe("ratingsOf", () => {
  it("splits your rating from the others' for one dish", () => {
    const all = [r("curry", "me", "love"), r("curry", "you", "nope"), r("soup", "me", "fine")];
    expect(ratingsOf(all, "curry", "me")).toEqual({ mine: "love", theirs: [r("curry", "you", "nope")] });
    expect(ratingsOf(all, "soup", "me")).toEqual({ mine: "fine", theirs: [] });
    expect(ratingsOf(all, "pasta", "me")).toEqual({ mine: undefined, theirs: [] });
  });
});
