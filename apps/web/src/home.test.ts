import { describe, expect, it } from "vitest";
import { shortName } from "./home";

describe("shortName", () => {
  it.each([
    ["Wohnzimmer Stehlampe", "Wohnzimmer", "Stehlampe"],
    ["wohnzimmer stehlampe", "Wohnzimmer", "stehlampe"],
    ["Wohnzimmer - Stehlampe", "Wohnzimmer", "Stehlampe"],
    ["Schlafzimmer: Nachttisch links", "Schlafzimmer", "Nachttisch links"],
    ["Küche Deckenlicht", "Küche", "Deckenlicht"],
    ["Living Room Floor Lamp", "Living Room", "Floor Lamp"],
  ])("strips %s in %s", (name, room, want) => {
    expect(shortName(name, room)).toBe(want);
  });

  it.each([
    ["Küchenlicht", "Küche"],
    ["Wohnzimmer", "Wohnzimmer"],
    ["Wohnzimmer -", "Wohnzimmer"],
    ["Stehlampe Wohnzimmer", "Wohnzimmer"],
    ["Stehlampe", "Flur"],
  ])("keeps %s in %s", (name, room) => {
    expect(shortName(name, room)).toBe(name);
  });

  it("keeps the name without a room", () => {
    expect(shortName("Wohnzimmer Stehlampe")).toBe("Wohnzimmer Stehlampe");
  });
});
