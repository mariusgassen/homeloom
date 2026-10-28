import type { ReactNode } from "react";
import { cx } from "./Icon";

export function Card({ eyebrow, title, children, footer, tone }: {
  eyebrow?: ReactNode;
  title?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
  tone?: "sunken";
}) {
  return (
    <article className={cx("hl-card", tone === "sunken" && "hl-card-sunken")}>
      {eyebrow && <div className="hl-eyebrow">{eyebrow}</div>}
      {title && <h3 className="hl-card-title">{title}</h3>}
      {children && <p className="hl-card-body">{children}</p>}
      {footer && <div className="hl-card-footer">{footer}</div>}
    </article>
  );
}
