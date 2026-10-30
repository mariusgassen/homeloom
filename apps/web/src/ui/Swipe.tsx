import { useEffect, useRef, useState, type PointerEvent, type ReactNode } from "react";
import { cx, Icon, type IconName } from "./Icon";

export type SwipeAction = { label: string; icon: IconName; tone?: "done" | "danger"; onAction: () => void };

// An action's width when the row is held open.
const actionWidth = 88;
// How far a drag has to go before it counts as sideways rather than a scroll.
const slop = 8;
// Swiping further than this share of the row does the action at once.
const full = 0.5;
// Opening one row closes the others.
const rows = new EventTarget();

// A list row with an action on each side, as in iOS Mail: swipe right for `start` (done), left for `end` (delete).
// A short swipe holds the action open to tap; a long one does it at once. Tapping the row, or anywhere else, closes it.
export function Swipe({ start, end, className, children }: { start?: SwipeAction; end?: SwipeAction; className?: string; children: ReactNode }) {
  const [dx, setDx] = useState(0);
  const [dragging, setDragging] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x: number; y: number; from: number; axis?: "x" | "y" } | null>(null);
  const swiped = useRef(false);

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

  if (!start && !end) return <div className={className}>{children}</div>;

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
    // Only towards a side that has an action.
    setDx(Math.min(start ? Infinity : 0, Math.max(end ? -Infinity : 0, d.from + x)));
  }

  function up() {
    const d = drag.current;
    drag.current = null;
    if (d?.axis !== "x") return;
    setDragging(false);
    const width = root.current?.offsetWidth ?? 0;
    const action = dx > 0 ? start : end;
    if (action && Math.abs(dx) > width * full) {
      setDx(0);
      action.onAction();
    } else setDx(Math.abs(dx) > actionWidth / 2 ? Math.sign(dx) * actionWidth : 0);
  }

  const side = (a: SwipeAction | undefined, where: "start" | "end") =>
    a && (
      <button
        type="button"
        className={cx("swipe-action", `swipe-action-${where}`, a.tone && `swipe-action-${a.tone}`)}
        style={{ width: Math.max(actionWidth, Math.abs(dx)) }}
        hidden={where === "start" ? dx <= 0 : dx >= 0}
        onClick={() => {
          setDx(0);
          a.onAction();
        }}
      >
        <Icon name={a.icon} />
        <span>{a.label}</span>
      </button>
    );

  return (
    <div ref={root} className={cx("swipe", className)}>
      {side(start, "start")}
      {side(end, "end")}
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
