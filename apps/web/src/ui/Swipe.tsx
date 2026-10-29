import { useEffect, useRef, useState, type PointerEvent, type ReactNode } from "react";
import { cx, Icon, type IconName } from "./Icon";

export type SwipeAction = { label: string; icon: IconName; tone?: "done" | "danger"; onAction: () => void };

// Each action's width when the row is open.
const actionWidth = 76;
// How far a drag has to go before it counts as sideways rather than a scroll.
const slop = 8;
// Opening one row closes the others.
const rows = new EventTarget();

// A list row that shows its actions when swiped left, as in iOS Mail. Tapping the row, or anywhere else, closes it.
// Without actions it is just the row.
export function Swipe({ actions, className, children }: { actions: SwipeAction[]; className?: string; children: ReactNode }) {
  const [dx, setDx] = useState(0);
  const [dragging, setDragging] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x: number; y: number; from: number; axis?: "x" | "y" } | null>(null);
  const swiped = useRef(false);
  const max = actions.length * actionWidth;

  useEffect(() => {
    const close = (e: Event) => (e as CustomEvent).detail !== root.current && setDx(0);
    rows.addEventListener("open", close);
    return () => rows.removeEventListener("open", close);
  }, []);

  useEffect(() => {
    if (dx === 0) return;
    const outside = (e: globalThis.PointerEvent) => !root.current?.contains(e.target as Node) && setDx(0);
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, [dx]);

  if (actions.length === 0) return <div className={className}>{children}</div>;

  function down(e: PointerEvent<HTMLDivElement>) {
    drag.current = { x: e.clientX, y: e.clientY, from: dx };
    swiped.current = false;
  }

  function move(e: PointerEvent<HTMLDivElement>) {
    const d = drag.current;
    if (!d) return;
    const x = e.clientX - d.x;
    const y = e.clientY - d.y;
    if (!d.axis) {
      if (Math.abs(x) > slop && Math.abs(x) > Math.abs(y)) {
        d.axis = "x";
        e.currentTarget.setPointerCapture(e.pointerId);
        rows.dispatchEvent(new CustomEvent("open", { detail: root.current }));
        setDragging(true);
      } else if (Math.abs(y) > slop) d.axis = "y";
    }
    if (d.axis !== "x") return;
    swiped.current = true;
    // Past fully open it gives a little, then stops.
    const next = Math.min(0, d.from + x);
    setDx(next < -max ? -max - (-max - next) / 4 : next);
  }

  function up() {
    const d = drag.current;
    drag.current = null;
    if (d?.axis !== "x") return;
    setDragging(false);
    setDx((at) => (at < -max / 2 ? -max : 0));
  }

  return (
    <div ref={root} className={cx("swipe", className)}>
      <div className="swipe-actions" style={{ width: max }} hidden={dx === 0 && !dragging}>
        {actions.map((a) => (
          <button
            key={a.label}
            type="button"
            className={cx("swipe-action", a.tone && `swipe-action-${a.tone}`)}
            onClick={() => {
              setDx(0);
              a.onAction();
            }}
          >
            <Icon name={a.icon} />
            <span>{a.label}</span>
          </button>
        ))}
      </div>
      <div
        className={cx("swipe-content", dragging && "swipe-dragging")}
        style={dx ? { transform: `translateX(${dx}px)` } : undefined}
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={up}
        onClickCapture={(e) => {
          // A swipe isn't a tap, and a tap on an open row only closes it.
          if (!swiped.current && dx === 0) return;
          e.preventDefault();
          e.stopPropagation();
          if (swiped.current) swiped.current = false;
          else setDx(0);
        }}
      >
        {children}
      </div>
    </div>
  );
}
