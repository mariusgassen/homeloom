import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router";
import { api, type MealRecap as Recap } from "../api/client";
import { useRefresh } from "../refresh";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { useHouseholdId } from "./Cook";
import { Screen } from "./Screen";

// A year of meal memories: how many, the dishes cooked most, the ones you both loved, and photos.
export function MealRecap() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const householdId = useHouseholdId();
  const [year, setYear] = useState(new Date().getFullYear());
  const [recap, setRecap] = useState<Recap | null>(null);
  const [failed, setFailed] = useState(false);
  const { generation, track } = useRefresh();

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/meal-recap", { params: { path: { householdId }, query: { year } } })
      .catch(() => undefined)
      .then((res) => {
        if (!current) return;
        setRecap(res?.data ?? null);
        setFailed(!res?.data);
      });
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, year, generation, track]);

  const thisYear = new Date().getFullYear();
  return (
    <Screen
      title={t("recap.title")}
      back={
        <Button variant="plain" size="sm" className="back" onClick={() => navigate("/cook")}>
          <span className="back-icon">
            <Icon name="chevron" size="sm" />
          </span>
          {t("cook.title")}
        </Button>
      }
    >
      <div className="recap-year">
        <Button variant="plain" size="sm" aria-label={t("recap.earlier")} onClick={() => setYear(year - 1)}>
          <span className="recap-prev">
            <Icon name="chevron" size="sm" />
          </span>
        </Button>
        <h2 className="title">{year}</h2>
        <Button variant="plain" size="sm" aria-label={t("recap.later")} disabled={year >= thisYear} onClick={() => setYear(year + 1)}>
          <Icon name="chevron" size="sm" />
        </Button>
      </div>
      {failed && (
        <p className="callout error" role="alert">
          {t("cook.failed")}
        </p>
      )}
      {recap && recap.memories === 0 && <Empty title={t("recap.empty.title")}>{t("recap.empty.body")}</Empty>}
      {recap && recap.memories > 0 && (
        <>
          <p className="body">{t("recap.memories", { count: recap.memories })}</p>
          {recap.dishes.length > 0 && (
            <section className="group">
              <h2 className="label group-title">{t("recap.dishes")}</h2>
              <div className="hl-list">
                {recap.dishes.map((d) => (
                  <Link key={d.slug} to={`/cook/${d.slug}`} className="card-link">
                    <ListRow title={d.name} subtitle={t("recap.times", { count: d.times })} trailing={<Icon name="chevron" size="sm" />} />
                  </Link>
                ))}
              </div>
            </section>
          )}
          {recap.favourites.length > 0 && (
            <section className="group">
              <h2 className="label group-title">{t("recap.favourites")}</h2>
              <div className="hl-list">
                {recap.favourites.map((d) => (
                  <Link key={d.slug} to={`/cook/${d.slug}`} className="card-link">
                    <ListRow title={d.name} leading={<Icon name="heart" size="sm" />} trailing={<Icon name="chevron" size="sm" />} />
                  </Link>
                ))}
              </div>
            </section>
          )}
          {recap.photos.length > 0 && (
            <section className="group">
              <h2 className="label group-title">{t("recap.photos")}</h2>
              <div className="recap-photos">
                {recap.photos.map((m) => (
                  <img key={m.id} className="recap-photo" src={m.photoUrl} alt={m.meal?.name ?? ""} loading="lazy" />
                ))}
              </div>
            </section>
          )}
        </>
      )}
    </Screen>
  );
}
