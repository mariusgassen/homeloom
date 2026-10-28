import { useId } from "react";
import { cx } from "./Icon";

// The two threads of the Homeloom mark (the gable, on a 512 grid): saffron passes under berry
// at the roof peak, cut by a mask so the gap is real transparency on any background.
const saffron = "M 328 116 L 136 276 L 136 404";
const berry = "M 184 116 L 376 276 L 376 404";

const clamp = (n: number) => Math.min(1, Math.max(0, n));

// Mark draws the mark. progress (0 to 1) draws the threads in one after the other, for a pull;
// weaving loops them for as long as something loads.
export function Mark({ size = 48, progress, weaving, label }: { size?: number; progress?: number; weaving?: boolean; label?: string }) {
  const id = useId();
  const drawn = (from: number) => (progress === undefined ? undefined : { strokeDashoffset: 1 - clamp((progress - from) * 2) });
  return (
    <svg
      viewBox="0 0 512 512"
      width={size}
      height={size}
      className={cx("hl-mark", progress !== undefined && "hl-mark-drawn", weaving && "hl-mark-weaving")}
      aria-hidden={label ? undefined : true}
      role={label ? "img" : undefined}
      aria-label={label}
    >
      <mask id={`${id}m`} maskUnits="userSpaceOnUse" x="0" y="0" width="512" height="512">
        <rect width="512" height="512" fill="#fff" />
        <clipPath id={`${id}c`}>
          <circle cx="256" cy="176" r="56" />
        </clipPath>
        <g clipPath={`url(#${id}c)`}>
          <path d={berry} stroke="#000" strokeWidth={84} />
        </g>
      </mask>
      <path className="hl-mark-a" d={saffron} pathLength={1} mask={`url(#${id}m)`} style={drawn(0)} />
      <path className="hl-mark-b" d={berry} pathLength={1} style={drawn(0.5)} />
    </svg>
  );
}
