import { useTranslation } from "react-i18next";

// Tappable titles used before, under a text field.
export function Suggestions({ items, onPick }: { items: string[]; onPick: (text: string) => void }) {
  const { t } = useTranslation();
  if (items.length === 0) return null;
  return (
    <div className="cook-chips suggestions" role="group" aria-label={t("entries.suggestions")}>
      {items.map((s) => (
        <button key={s} type="button" className="hl-chip" onClick={() => onPick(s)}>
          {s}
        </button>
      ))}
    </div>
  );
}
