import { NavLink } from "react-router";
import { Icon, type IconName } from "./Icon";

// The design system's TabBar, with router links instead of buttons so tabs are real URLs.
export function TabBar({ items, label }: { items: { to: string; label: string; icon: IconName }[]; label: string }) {
  return (
    <nav className="hl-tabbar" aria-label={label}>
      {items.map((it) => (
        <NavLink key={it.to} to={it.to} end={it.to === "/"} className="hl-tab">
          <span className="hl-tab-mark">
            <Icon name={it.icon} />
          </span>
          <span>{it.label}</span>
        </NavLink>
      ))}
    </nav>
  );
}
