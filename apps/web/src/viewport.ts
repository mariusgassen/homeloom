import { useEffect, useState } from "react";

// iOS keeps the layout viewport full height when the keyboard opens and pans it instead, so fixed sheets
// end up half behind the keyboard. This publishes the visible area as --vv-top and --vv-height, which
// sheets and the assistant use to sit above the keyboard, and keeps the focused field in view.
export function trackViewport(win: Window = window) {
  const vv = win.visualViewport;
  if (!vv) return () => {};
  const root = win.document.documentElement;
  let frame = 0;
  const update = () => {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(() => {
      root.style.setProperty("--vv-top", `${vv.offsetTop}px`);
      root.style.setProperty("--vv-height", `${vv.height}px`);
      root.classList.toggle("keyboard", win.innerHeight - vv.height > 120);
      const field = win.document.activeElement;
      if (field instanceof HTMLElement && field.matches("input, textarea") && field.closest(".sheet, .assistant")) {
        field.scrollIntoView({ block: "nearest" });
      }
    });
  };
  update();
  vv.addEventListener("resize", update);
  vv.addEventListener("scroll", update);
  return () => {
    cancelAnimationFrame(frame);
    vv.removeEventListener("resize", update);
    vv.removeEventListener("scroll", update);
  };
}

// True from the desktop breakpoint up (matches styles/desktop.css), where lists open their detail beside them.
export function useDesktop() {
  const query = "(min-width: 1024px)";
  const [wide, setWide] = useState(() => window.matchMedia?.(query).matches ?? false);
  useEffect(() => {
    const m = window.matchMedia?.(query);
    if (!m) return;
    const on = () => setWide(m.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return wide;
}
