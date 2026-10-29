import { useId, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { cx, Icon, type IconName } from "./Icon";

// Form controls for sheets, in place of dropdowns: one tap picks, and every choice stays visible.

type Option<T extends string> = { value: T; label: string; icon?: IconName };

// A few short, equal choices side by side.
export function Segmented<T extends string>({ label, value, options, onChange, disabled }: {
  label: string;
  value: T;
  options: Option<T>[];
  onChange: (v: T) => void;
  disabled?: boolean;
}) {
  return (
    <div className="segmented" role="radiogroup" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={value === o.value} disabled={disabled} onClick={() => onChange(o.value)}>
          {o.icon && <Icon name={o.icon} size="sm" />}
          {o.label}
        </button>
      ))}
    </div>
  );
}

// More or longer choices: a row of pills that scrolls sideways.
export function Pills<T extends string>({ label, value, options, onChange }: {
  label: string;
  value: T;
  options: Option<T>[];
  onChange: (v: T) => void;
}) {
  return (
    <div className="pills" role="radiogroup" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={value === o.value} className="pill" onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

// Rows in one rounded group, like an iOS settings table.
export function FormGroup({ children }: { children: ReactNode }) {
  return <div className="form-group">{children}</div>;
}

// A labelled row whose control sits below the label, for pills and segments inside a group.
export function FormBlock({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="form-block">
      <span className="form-row-label">{label}</span>
      {children}
    </div>
  );
}

export function SwitchRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <button type="button" className="form-row" role="switch" aria-checked={checked} onClick={() => onChange(!checked)}>
      <span className="form-row-label">{label}</span>
      <span className={cx("switch", checked && "switch-on")} aria-hidden="true" />
    </button>
  );
}

// A date or date and time as readable text; the native picker opens on a tap. Empty shows the placeholder,
// and an optional field can be cleared again.
export function DateRow({ label, type, value, onChange, placeholder, min, max, required }: {
  label: string;
  type: "date" | "datetime-local";
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  min?: string;
  max?: string;
  required?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const shown = value
    ? new Intl.DateTimeFormat(i18n.language, {
        weekday: "short",
        day: "numeric",
        month: "short",
        ...(type === "datetime-local" && { hour: "numeric", minute: "2-digit" }),
      }).format(new Date(type === "date" ? `${value}T00:00` : value))
    : placeholder;
  return (
    <div className="form-row">
      <span id={id} className="form-row-label">
        {label}
      </span>
      <span className={cx("date-pill", !value && "date-pill-empty")}>
        {shown}
        <input
          type={type}
          aria-labelledby={id}
          value={value}
          min={min}
          max={max}
          required={required}
          onChange={(e) => onChange(e.target.value)}
          onClick={(e) => {
            try {
              e.currentTarget.showPicker();
            } catch {
              // iOS opens its picker on the tap anyway.
            }
          }}
        />
      </span>
      {!required && value && (
        <button type="button" className="form-clear" aria-label={t("entries.clear")} onClick={() => onChange("")}>
          <Icon name="close" size="sm" />
        </button>
      )}
    </div>
  );
}
