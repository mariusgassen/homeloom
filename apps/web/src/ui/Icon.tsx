// Line icons on a 24px grid, 1.75px stroke, from the Homeloom design system.
const paths = {
  today: ["M4 11.5 12 5l8 6.5", "M6 10v9h12v-9", "M10 19v-5h4v5"],
  cook: ["M4 11h16", "M5 11v6a3 3 0 0 0 3 3h8a3 3 0 0 0 3-3v-6", "M9 7c0-1.5 1-1.5 1-3", "M14 7c0-1.5 1-1.5 1-3"],
  us: [
    "M8.5 6a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7Z",
    "M15.5 6a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7Z",
    "M3 20c.8-3 3-4.5 5.5-4.5S13 17 14 20",
    "M13.5 15.8c.6-.2 1.3-.3 2-.3 2.5 0 4.7 1.5 5.5 4.5",
  ],
  lists: ["M9 6h11", "M9 12h11", "M9 18h11", "M4 6l1 1 2-2", "M4 12l1 1 2-2", "M4.5 18h1"],
  home: ["M3 12h4l2-3 2 6 2-9 2 6h6"],
  lock: ["M7 11V8a5 5 0 0 1 10 0v3", "M5.5 11h13v9h-13z", "M12 15v2"],
  shared: ["M9 7.5a4.5 4.5 0 1 0 0 9 4.5 4.5 0 0 0 0-9Z", "M15 7.5a4.5 4.5 0 1 0 0 9 4.5 4.5 0 0 0 0-9Z"],
  reveal: ["M7 4h10", "M7 20h10", "M8 4c0 4 8 4 8 8s-8 4-8 8", "M16 4c0 4-8 4-8 8s8 4 8 8"],
  chevron: ["M9 6l6 6-6 6"],
  plus: ["M12 5v14", "M5 12h14"],
  close: ["M6 6l12 12", "M18 6 6 18"],
  check: ["M5 12.5l4.5 4.5L19 7.5"],
  calendar: ["M5 6h14v14H5z", "M5 10h14", "M9 4v4", "M15 4v4"],
  bell: ["M6 16V11a6 6 0 0 1 12 0v5l1.5 2h-15z", "M10 20.5a2 2 0 0 0 4 0"],
  send: ["M12 19V5", "M6 11l6-6 6 6"],
  sparkle: ["M12 4v4", "M12 16v4", "M4 12h4", "M16 12h4", "M12 8.5l1.2 2.3 2.3 1.2-2.3 1.2L12 15.5l-1.2-2.3-2.3-1.2 2.3-1.2z"],
} as const;

export type IconName = keyof typeof paths;

export function Icon({ name, size, label }: { name: IconName; size?: "sm"; label?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      className={cx("hl-icon", size === "sm" && "hl-icon-sm")}
      aria-hidden={label ? undefined : true}
      role={label ? "img" : undefined}
      aria-label={label}
    >
      {paths[name].map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  );
}

export function cx(...names: (string | false | undefined)[]) {
  return names.filter(Boolean).join(" ");
}
