import { describe, expect, it } from "vitest";
import { greeting, instructions, parseCaller } from "./caller.js";

describe("parseCaller", () => {
  it("reads the dispatch metadata", () => {
    expect(parseCaller('{"key":"hla_x","lang":"de","name":"Alice Doe"}')).toEqual({ key: "hla_x", lang: "de", name: "Alice Doe" });
  });

  it("falls back to English and needs a key", () => {
    expect(parseCaller('{"key":"hla_x","lang":"fr"}')).toEqual({ key: "hla_x", lang: "en", name: "" });
    expect(() => parseCaller('{"lang":"de"}')).toThrow();
  });
});

describe("speech", () => {
  const alice = { key: "hla_secret", lang: "de" as const, name: "Alice Doe" };

  it("greets the caller by first name in their language", () => {
    expect(greeting(alice)).toBe("Hi Alice, was kann ich für dich tun?");
    expect(greeting({ ...alice, lang: "en", name: "" })).toBe("Hi, what can I do for you?");
  });

  it("tells the model the language and the household's time", () => {
    const text = instructions(alice, new Date("2026-10-28T17:30:00Z"), "Europe/Berlin");
    expect(text).toContain("German, using informal du");
    expect(text).toContain("18:30");
    expect(text).not.toContain(alice.key);
  });
});
