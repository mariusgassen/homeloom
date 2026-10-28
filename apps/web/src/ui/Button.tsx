import type { ButtonHTMLAttributes } from "react";
import { cx, Icon, type IconName } from "./Icon";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "plain" | "danger";
  size?: "sm";
  block?: boolean;
  icon?: IconName;
};

export function Button({ variant = "primary", size, block, icon, className, children, ...rest }: Props) {
  return (
    <button
      type="button"
      {...rest}
      className={cx("hl-btn", `hl-btn-${variant}`, size === "sm" && "hl-btn-sm", block && "hl-btn-block", className)}
    >
      {icon && <Icon name={icon} size={size === "sm" ? "sm" : undefined} />}
      {children}
    </button>
  );
}
