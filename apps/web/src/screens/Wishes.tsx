import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { api, type Wish } from "../api/client";
import { useMoney } from "../money";
import { useSession } from "../session";
import { heartsOf, setGot, useWishes, vote } from "../wishes";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { cx, Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { Swipe } from "../ui/Swipe";
import { Screen } from "./Screen";
import { WishSheet } from "./WishSheet";

// The wishlist, opened from Today: what you'd like for the home, the most wanted first. Each of
// you gives a wish one to three hearts; what came true moves to the end.
export function Wishes() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const money = useMoney();
  const { session } = useSession();
  if (session.state !== "signedIn" || !session.household)
    throw new Error("wishes without a household");
  const { user: me, household } = session;
  const { wishes, failed, reload } = useWishes(household.id);
  const [sheet, setSheet] = useState<Wish | "new" | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function run(job: Promise<string | null>) {
    const err = await job;
    setError(err === null ? null : err || t("wishes.failed"));
    reload();
  }

  async function remove(w: Wish) {
    if (!window.confirm(t("wishes.deleteConfirm"))) return;
    const res = await api
      .DELETE("/v1/households/{householdId}/wishes/{wishId}", {
        params: { path: { householdId: household.id, wishId: w.id } },
      })
      .catch(() => undefined);
    setError(
      res?.response.ok ? null : (res?.error?.message ?? t("wishes.failed")),
    );
    reload();
  }

  // What the others gave a wish, as "Anna ♥♥".
  const others = (w: Wish) =>
    w.votes
      .filter((v) => v.userId !== me.id)
      .map((v) =>
        `${household.members.find((m) => m.user.id === v.userId)?.user.name ?? ""} ${"♥".repeat(v.hearts)}`.trim(),
      );
  const host = (url: string) => {
    try {
      return new URL(url).hostname.replace(/^www\./, "");
    } catch {
      return "";
    }
  };

  const row = (w: Wish) => {
    const done = Boolean(w.gotAt);
    const subtitle = [
      w.priceCents ? money(w.priceCents) : "",
      w.url ? host(w.url) : "",
      ...others(w),
    ]
      .filter(Boolean)
      .join(" · ");
    return (
      <Swipe
        key={w.id}
        start={{
          label: t(done ? "wishes.again" : "wishes.got"),
          icon: done ? "close" : "check",
          tone: done ? "danger" : "done",
          onAction: () => void run(setGot(household.id, w, !done)),
        }}
        end={{
          label: t("wishes.delete"),
          icon: "trash",
          tone: "danger",
          onAction: () => void remove(w),
        }}
      >
        <div className={cx("wish-row", done && "wish-got")}>
          <ListRow
            leading={
              w.photoUrl ? (
                <img
                  className="today-tile-photo"
                  src={w.photoUrl}
                  alt=""
                  loading="lazy"
                />
              ) : (
                <span className="today-tile-icon">
                  <Icon name="sofa" />
                </span>
              )
            }
            title={w.title}
            subtitle={subtitle || undefined}
            onClick={() => setSheet(w)}
          />
          {!done && (
            <Hearts
              value={heartsOf(w, me.id)}
              label={w.title}
              onChange={(n) => void run(vote(household.id, w, n))}
            />
          )}
        </div>
      </Swipe>
    );
  };

  const open = wishes?.filter((w) => !w.gotAt) ?? [];
  const got = wishes?.filter((w) => w.gotAt) ?? [];
  return (
    <Screen
      title={t("wishes.title")}
      back={
        <Button
          variant="plain"
          size="sm"
          className="back"
          onClick={() => navigate("/us?show=together")}
        >
          <span className="back-icon">
            <Icon name="chevron" size="sm" />
          </span>
          {t("tabs.us")}
        </Button>
      }
      add={{ label: t("wishes.new"), onClick: () => setSheet("new") }}
    >
      {(failed || error) && (
        <p className="callout error" role="alert">
          {error ?? t("wishes.failed")}
        </p>
      )}
      {wishes?.length === 0 && (
        <Empty
          title={t("wishes.emptyTitle")}
          action={
            <Button size="sm" onClick={() => setSheet("new")}>
              {t("wishes.new")}
            </Button>
          }
        >
          {t("wishes.emptyBody")}
        </Empty>
      )}
      {open.length > 0 && (
        <section className="group">
          <h2 className="label group-title">{t("wishes.wanted")}</h2>
          <div className="hl-list">{open.map(row)}</div>
        </section>
      )}
      {got.length > 0 && (
        <section className="group">
          <h2 className="label group-title">{t("wishes.cameTrue")}</h2>
          <div className="hl-list">{got.map(row)}</div>
        </section>
      )}
      {sheet && (
        <WishSheet
          household={household}
          wish={sheet === "new" ? undefined : sheet}
          onClose={() => setSheet(null)}
          onSaved={() => {
            setSheet(null);
            reload();
          }}
        />
      )}
    </Screen>
  );
}

// Your hearts for a wish: tap the one you mean, tap your last heart again to take them back.
function Hearts({
  value,
  label,
  onChange,
}: {
  value: number;
  label: string;
  onChange: (hearts: number) => void;
}) {
  const { t } = useTranslation();
  return (
    <span
      className="wish-hearts"
      role="group"
      aria-label={t("wishes.hearts", { wish: label })}
    >
      {[1, 2, 3].map((n) => (
        <button
          key={n}
          type="button"
          className={cx(
            "screen-action",
            "wish-heart",
            n <= value && "wish-heart-on",
          )}
          aria-pressed={n <= value}
          aria-label={t("wishes.heartsCount", { count: n })}
          onClick={() => onChange(n === value ? 0 : n)}
        >
          <Icon name="heart" size="sm" />
        </button>
      ))}
    </span>
  );
}
