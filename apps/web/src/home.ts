// shortName drops a leading room name from a device name, so "Wohnzimmer Stehlampe" in the room
// "Wohnzimmer" reads "Stehlampe". Display only: it never returns an empty name and only matches
// whole words, so "Küchenlicht" stays in the room "Küche".
export function shortName(name: string, room?: string): string {
  const prefix = room?.trim();
  if (!prefix) return name;
  const start = name.trimStart();
  if (start.length <= prefix.length || start.slice(0, prefix.length).toLocaleLowerCase() !== prefix.toLocaleLowerCase()) return name;
  const rest = start.slice(prefix.length);
  const separators = /^[\s\-–—:_.·|/]+/;
  const match = rest.match(separators);
  if (!match) return name;
  const short = rest.slice(match[0].length).trim();
  return short || name;
}
