import { cx } from "./Icon";

// Each person in a household has a thread colour: the first saffron, the second berry.
export type Thread = "saffron" | "berry";

export function threadFor(index: number): Thread | undefined {
  return (["saffron", "berry"] as const)[index];
}

function initials(name: string) {
  return (
    name
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((s) => s.charAt(0).toUpperCase())
      .join("") || "?"
  );
}

export function ThreadAvatar({ name, src, thread, size }: { name: string; src?: string; thread?: Thread; size?: "sm" | "lg" }) {
  return (
    <span className={cx("hl-avatar", thread && `hl-avatar-${thread}`, size && `hl-avatar-${size}`)} role="img" aria-label={name}>
      {src ? <img src={src} alt="" /> : initials(name)}
    </span>
  );
}
