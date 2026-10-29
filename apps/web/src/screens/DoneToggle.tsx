import { useTranslation } from "react-i18next";
import { Button } from "../ui/Button";

// Shows or hides a list's done items, saying how many there are.
export function DoneToggle({ count, shown, onToggle }: { count: number; shown: boolean; onToggle: () => void }) {
  const { t } = useTranslation();
  return (
    <Button variant="plain" size="sm" aria-expanded={shown} onClick={onToggle}>
      {t(shown ? "entries.hideDone" : "entries.showDone", { count })}
    </Button>
  );
}
