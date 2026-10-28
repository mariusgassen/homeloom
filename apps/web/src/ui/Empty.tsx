import type { ReactNode } from "react";
import { Mark } from "./Mark";

// An empty list or tab: the mark, a short title, a line of help and, if there is one, the way on.
export function Empty({ title, children, action }: { title: ReactNode; children?: ReactNode; action?: ReactNode }) {
  return (
    <section className="hl-empty">
      <Mark size={40} />
      <h3 className="headline">{title}</h3>
      {children && <p className="callout muted">{children}</p>}
      {action}
    </section>
  );
}
