import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { HouseholdDetail, Rating, Recipe, User } from "../api/client";
import { ratingsOf, ratings as order, useTaste } from "../taste";
import { Button } from "../ui/Button";
import { cx, Icon } from "../ui/Icon";
import { MomentSheet } from "./MomentSheet";

// How each of you liked a dish, and a way to keep a memory of cooking it.
export function RecipeMemory({ household, me, recipe }: { household: HouseholdDetail; me: User; recipe: Recipe }) {
  const { t } = useTranslation();
  const { all, rate } = useTaste(household.id);
  const [error, setError] = useState<string | null>(null);
  const [sheet, setSheet] = useState(false);
  const [kept, setKept] = useState(false);
  const { mine, theirs } = ratingsOf(all ?? [], recipe.slug, me.id);
  const partner = household.members.find((m) => m.user.id !== me.id)?.user;

  async function pick(rating: Rating) {
    setError(null);
    const failed = await rate(recipe.slug, me.id, mine === rating ? null : rating);
    if (failed !== null) setError(failed || t("cook.failed"));
  }

  return (
    <section className="group" aria-label={t("taste.title")}>
      <h2 className="label group-title">{t("taste.title")}</h2>
      <div className="cook-chips" role="group" aria-label={t("taste.title")}>
        {order.map((r) => (
          <button key={r} type="button" className={cx("hl-chip", mine === r && "hl-chip-shared")} aria-pressed={mine === r} onClick={() => pick(r)}>
            {r === "love" && <Icon name="heart" size="sm" />}
            {t(`taste.rating.${r}`)}
          </button>
        ))}
      </div>
      {theirs.map((r) => (
        <p key={r.userId} className="caption muted">
          {t(`taste.theirs.${r.rating}`, { name: partner?.name ?? "" })}
        </p>
      ))}
      {error && (
        <p className="callout error" role="alert">
          {error}
        </p>
      )}
      {kept ? (
        <p className="callout success" role="status">
          {t("taste.kept")}
        </p>
      ) : (
        <Button block variant="secondary" icon="camera" onClick={() => setSheet(true)}>
          {t("taste.cooked")}
        </Button>
      )}
      {sheet && (
        <MomentSheet
          household={household}
          me={me}
          meal={{ slug: recipe.slug, name: recipe.name }}
          onClose={() => setSheet(false)}
          onSaved={() => {
            setSheet(false);
            setKept(true);
          }}
        />
      )}
    </section>
  );
}
