import type { ReactNode } from "react";
import { Icon } from "./Icon";

export function ListRow({ title, subtitle, leading, trailing, onClick }: {
  title: ReactNode;
  subtitle?: ReactNode;
  leading?: ReactNode;
  trailing?: ReactNode;
  onClick?: () => void;
}) {
  const content = (
    <>
      {leading}
      <span className="hl-listrow-main">
        <span className="hl-listrow-title">{title}</span>
        {subtitle && <span className="hl-listrow-sub">{subtitle}</span>}
      </span>
      {(trailing || onClick) && (
        <span className="hl-listrow-trail">
          {trailing}
          {onClick && <Icon name="chevron" size="sm" />}
        </span>
      )}
    </>
  );
  return onClick ? (
    <button type="button" className="hl-listrow" onClick={onClick}>
      {content}
    </button>
  ) : (
    <div className="hl-listrow">{content}</div>
  );
}
