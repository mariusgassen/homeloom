import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router";
import { api, type Wish, type WishKind } from "../api/client";
import { useShowDone } from "../entries";
import { local } from "../outbox";
import { useRefresh } from "../refresh";
import { useSession } from "../session";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { Swipe } from "../ui/Swipe";
import { DoneToggle } from "./DoneToggle";
import { Screen } from "./Screen";

const kinds: WishKind[] = ["watch", "read", "visit"];

// Things to watch, read or visit some day, shared by the household: add at the top, tick off
// when watched, read or been there.
export function Wishes() {
  const { t } = useTranslation();
  const { session } = useSession();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  if (session.state !== "signedIn" || !session.household) throw new Error("screen without a household");
  const { user: me, household } = session;
  const householdId = household.id;
  const kind = kinds.find((k) => k === params.get("show")) ?? "watch";
  const [wishes, setWishes] = useState<Wish[] | null>(() => local<Wish[]>(`/v1/households/${householdId}/wishes`) ?? null);
  const [failed, setFailed] = useState(false);
  const [text, setText] = useState("");
  const [generation, setGeneration] = useState(0);
  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  const { generation: refreshed, track } = useRefresh();
  const [showDone, toggleDone] = useShowDone(me.id, "wishes");

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/wishes", { params: { path: { householdId } } })
      .then((res) => current && (res.data ? (setWishes(res.data), setFailed(false)) : setFailed(true)))
      .catch(() => current && setFailed(true));
    track(request);
    return () => {
      current = false;
    };
  }, [householdId, generation, refreshed, track]);

  // Every change reloads the list, so what the other person did shows up too.
  async function run(call: Promise<{ error?: unknown }>) {
    const res = await call.catch(() => ({ error: true }));
    setFailed(!!res.error);
    reload();
    return !res.error;
  }

  async function add(e: FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    const body = { kind, text };
    if (await run(api.POST("/v1/households/{householdId}/wishes", { params: { path: { householdId } }, body }))) setText("");
  }

  const wish = (w: Wish) => ({ params: { path: { householdId, wishId: w.id } } });
  const tick = (w: Wish) => run(w.done ? api.DELETE("/v1/households/{householdId}/wishes/{wishId}/done", wish(w)) : api.PUT("/v1/households/{householdId}/wishes/{wishId}/done", wish(w)));
  const remove = (w: Wish) => run(api.DELETE("/v1/households/{householdId}/wishes/{wishId}", wish(w)));

  const mine = wishes?.filter((w) => w.kind === kind) ?? [];
  const open = mine.filter((w) => !w.done);
  const done = mine.filter((w) => w.done);
  const row = (w: Wish) => (
    <Swipe
      key={w.id}
      start={{ label: t(w.done ? "entries.reopen" : "entries.done"), icon: "check", tone: "done", onAction: () => void tick(w) }}
      end={{ label: t("entries.delete"), icon: "trash", tone: "danger", onAction: () => void remove(w) }}
    >
      <div className="entry-row">
        <button type="button" className={w.done ? "tick tick-done" : "tick"} role="checkbox" aria-checked={w.done} aria-label={w.text} onClick={() => tick(w)}>
          {w.done && <Icon name="check" size="sm" />}
        </button>
        <ListRow
          title={<span className={w.done ? "have" : undefined}>{w.text}</span>}
          trailing={
            <button type="button" className="screen-action" aria-label={t("wishes.remove", { item: w.text })} onClick={() => remove(w)}>
              <Icon name="close" size="sm" />
            </button>
          }
        />
      </div>
    </Swipe>
  );

  return (
    <Screen
      title={t("wishes.title")}
      back={
        <Button variant="plain" size="sm" className="back" onClick={() => navigate("/")}>
          <span className="back-icon">
            <Icon name="chevron" size="sm" />
          </span>
          {t("tabs.today")}
        </Button>
      }
    >
      <div className="segmented" role="tablist" aria-label={t("wishes.kind")}>
        {kinds.map((k) => (
          <button key={k} type="button" role="tab" aria-selected={kind === k} onClick={() => setParams({ show: k }, { replace: true })}>
            {t(`wishes.tabs.${k}`)}
          </button>
        ))}
      </div>
      <form className="shopping-add" onSubmit={add}>
        <input className="input" value={text} onChange={(e) => setText(e.target.value)} placeholder={t(`wishes.placeholder.${kind}`)} aria-label={t(`wishes.placeholder.${kind}`)} maxLength={200} enterKeyHint="done" />
        <Button type="submit" icon="plus" disabled={!text.trim()} aria-label={t("wishes.add")} />
      </form>
      {failed && (
        <p className="callout error" role="alert">
          {t("wishes.failed")}
        </p>
      )}
      {wishes && mine.length === 0 && <Empty title={t(`wishes.empty.${kind}.title`)}>{t(`wishes.empty.${kind}.body`)}</Empty>}
      {open.length > 0 && <div className="hl-list">{open.map(row)}</div>}
      {done.length > 0 && (
        <section className="group">
          <div className="shopping-done">
            <h2 className="label group-title">{t(`wishes.done.${kind}`)}</h2>
            <DoneToggle count={done.length} shown={showDone} onToggle={toggleDone} />
          </div>
          {showDone && <div className="hl-list">{done.map(row)}</div>}
        </section>
      )}
    </Screen>
  );
}
