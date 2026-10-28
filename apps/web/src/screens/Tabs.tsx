import { useTranslation } from "react-i18next";
import { Card } from "../ui/Card";
import { Screen } from "./Screen";

// The tabs hold empty states until their features land; each says what will live there.

export function Today() {
  const { t, i18n } = useTranslation();
  const date = new Intl.DateTimeFormat(i18n.language, { weekday: "long", day: "numeric", month: "long" }).format(new Date());
  return (
    <Screen title={t("tabs.today")} overline={date}>
      {(["question", "dinner", "due"] as const).map((k) => (
        <Card key={k} eyebrow={<span>{t(`today.${k}.eyebrow`)}</span>} title={t(`today.${k}.title`)}>
          {t(`today.${k}.body`)}
        </Card>
      ))}
    </Screen>
  );
}

function Placeholder({ tab }: { tab: "cook" | "us" | "lists" | "home" }) {
  const { t } = useTranslation();
  return (
    <Screen title={t(`tabs.${tab}`)}>
      <Card tone="sunken" title={t(`${tab}.title`)}>
        {t(`${tab}.body`)}
      </Card>
    </Screen>
  );
}

export const Cook = () => <Placeholder tab="cook" />;
export const Us = () => <Placeholder tab="us" />;
export const Lists = () => <Placeholder tab="lists" />;
export const Home = () => <Placeholder tab="home" />;
