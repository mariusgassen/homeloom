import { expect, test } from "vitest";
import { excerpt, firstPhoto, joinNote, splitNote } from "./notes";

const photo = "/v1/entries/0b6a3f4e-1c2d-4e5f-8a9b-0c1d2e3f4a5b/photos/1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f";

test("a note's title is its first-line heading, and joining undoes splitting", () => {
  expect(splitNote("# Trip ##\n\nPack light")).toEqual({ title: "Trip", body: "Pack light" });
  expect(splitNote("Pack light\nsocks")).toEqual({ title: "", body: "Pack light\nsocks" });
  expect(joinNote(" Trip ", "Pack light\n")).toBe("# Trip\n\nPack light");
  expect(joinNote("", "Pack light")).toBe("Pack light");
  expect(joinNote("Trip", "")).toBe("# Trip");
});

test("the list shows the first of the note's own photos and plain text below the title", () => {
  const text = `# Trip\n\n![](https://example.com/x.png)\n![](${photo})\n\n- **socks**\n- [map](https://maps.example)`;
  expect(firstPhoto(text)).toBe(photo);
  expect(firstPhoto("![](https://example.com/x.png)")).toBeUndefined();
  expect(excerpt(text)).toBe("socks\nmap");
});
