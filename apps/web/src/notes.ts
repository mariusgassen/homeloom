import { api, type Entry } from "./api/client";

// A note is Markdown; a "# Heading" on its first line is its title. The API reads the title the same
// way, so notes apps that sync the files see the same name.
export function splitNote(text: string): { title: string; body: string } {
  const m = /^#[ \t]+(.*)(?:\r?\n|$)/.exec(text);
  if (!m) return { title: "", body: text };
  return { title: (m[1] ?? "").replace(/\s+#+\s*$/, "").trim(), body: text.slice(m[0].length).replace(/^\s*\n/, "") };
}

export function joinNote(title: string, body: string) {
  const t = title.trim();
  const b = body.trim();
  return t ? (b ? `# ${t}\n\n${b}` : `# ${t}`) : b;
}

// Photos live in the API, so a note only shows those it links there; nothing else is ever loaded.
export const photoUrl = /^\/v1\/entries\/[0-9a-f-]{36}\/photos\/[0-9a-f-]{36}$/;
const photoLinks = /!\[[^\]]*\]\(([^)\s]*)\)/g;

export function firstPhoto(text: string) {
  for (const m of text.matchAll(photoLinks)) if (m[1] && photoUrl.test(m[1])) return m[1];
  return undefined;
}

// excerpt is what a note says below its title, without the Markdown, for the list.
export function excerpt(text: string) {
  const lines = text.replace(photoLinks, "").split("\n");
  const first = lines.findIndex((l) => l.trim() !== "");
  return lines
    .slice(first + 1)
    .map((l) => l.replace(/^\s*(?:#{1,6}\s+|>\s*|[-*+]\s+(?:\[[ xX]\]\s+)?|\d+[.)]\s+)/, "").replace(/\[([^\]]*)\]\([^)]*\)/g, "$1").replace(/[*`~]|__/g, ""))
    .filter((l) => l.trim() !== "")
    .join("\n");
}

export async function uploadNotePhoto(note: Entry, photo: Blob) {
  const res = await api
    .POST("/v1/entries/{entryId}/photos", {
      params: { path: { entryId: note.id } },
      // The contract types a binary body as a string; the photo goes as it is.
      body: photo as unknown as string,
      bodySerializer: (b) => b,
      headers: { "Content-Type": "image/jpeg" },
    })
    .catch(() => undefined);
  return res?.data;
}
