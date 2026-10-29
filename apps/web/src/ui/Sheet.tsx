import { useEffect, useRef, type FormEvent, type PointerEvent, type ReactNode } from "react";

// A bottom sheet: a grabber, a bar (leading action, title, trailing action), then the content. It sits above
// the keyboard (see viewport.ts), closes on a tap outside, Escape, or a swipe down on its top, and is a
// form when it gets onSubmit.
export function Sheet({ title, leading, trailing, onClose, onSubmit, children }: {
  title: string;
  leading?: ReactNode;
  trailing?: ReactNode;
  onClose: () => void;
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
  children: ReactNode;
}) {
  const sheet = useRef<HTMLElement>(null);
  const drag = useRef<{ id: number; y: number; dy: number } | null>(null);
  const close = useRef(onClose);
  useEffect(() => {
    close.current = onClose;
  });

  useEffect(() => {
    const key = (e: KeyboardEvent) => e.key === "Escape" && close.current();
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);

  function down(e: PointerEvent) {
    if ((e.target as HTMLElement).closest("button, a, input")) return;
    drag.current = { id: e.pointerId, y: e.clientY, dy: 0 };
    e.currentTarget.setPointerCapture(e.pointerId);
  }
  function move(e: PointerEvent) {
    const d = drag.current;
    if (!d || d.id !== e.pointerId || !sheet.current) return;
    d.dy = Math.max(0, e.clientY - d.y);
    sheet.current.style.transition = "none";
    sheet.current.style.transform = `translateY(${d.dy}px)`;
  }
  function up(e: PointerEvent) {
    const d = drag.current;
    if (!d || d.id !== e.pointerId || !sheet.current) return;
    drag.current = null;
    sheet.current.style.transition = "";
    sheet.current.style.transform = "";
    if (d.dy > 96) onClose();
  }

  const head = (
    <div className="sheet-head" onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerCancel={up}>
      <div className="sheet-grabber" aria-hidden="true" />
      <div className="sheet-bar">
        {leading}
        <h2 className="headline sheet-title">{title}</h2>
        {trailing}
      </div>
    </div>
  );
  return (
    <div className="sheet-backdrop" onClick={(e) => e.target === e.currentTarget && onClose()}>
      {onSubmit ? (
        <form ref={(el) => void (sheet.current = el)} className="sheet" role="dialog" aria-modal="true" aria-label={title} onSubmit={onSubmit}>
          {head}
          {children}
        </form>
      ) : (
        <div ref={(el) => void (sheet.current = el)} className="sheet" role="dialog" aria-modal="true" aria-label={title}>
          {head}
          {children}
        </div>
      )}
    </div>
  );
}
