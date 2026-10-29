import { describe, expect, it } from "vitest";
import { defaultVoice, greeting, instructions, parseCaller } from "./caller.js";

describe("parseCaller", () => {
  it("reads the dispatch metadata", () => {
    const voice = { voice: "marin", speed: 1.2, instructions: "Speak calmly." };
    expect(parseCaller(JSON.stringify({ key: "hla_x", e2ee: "k", lang: "de", name: "Alice Doe", voice }))).toEqual({ key: "hla_x", e2ee: "k", lang: "de", name: "Alice Doe", voice });
  });

  it("falls back to English and the default voice, and needs both keys", () => {
    expect(parseCaller('{"key":"hla_x","e2ee":"k","lang":"fr"}')).toEqual({ key: "hla_x", e2ee: "k", lang: "en", name: "", voice: defaultVoice });
    expect(parseCaller('{"key":"hla_x","e2ee":"k","voice":{"voice":"cedar","speed":"fast"}}').voice).toEqual({ ...defaultVoice, voice: "cedar" });
    expect(() => parseCaller('{"lang":"de"}')).toThrow();
    expect(() => parseCaller('{"key":"hla_x"}')).toThrow();
  });
});

describe("speech", () => {
  const alice = { key: "hla_secret", e2ee: "k", lang: "de" as const, name: "Alice Doe", voice: defaultVoice };

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
