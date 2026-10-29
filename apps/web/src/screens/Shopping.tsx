import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type ShoppingItem } from "../api/client";
import { matching, useShowDone, useSuggestions } from "../entries";
import { useRefresh } from "../refresh";
import { Button } from "../ui/Button";
import { Empty } from "../ui/Empty";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";
import { DoneToggle } from "./DoneToggle";
import { Suggestions } from "./Suggestions";

// The household's shared shopping list: add at the top, tick off, clear what's ticked.
export function Shopping({ householdId, userId }: { householdId: string; userId: string }) {
  const { t } = useTranslation();
  const [items, setItems] = useState<ShoppingItem[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [text, setText] = useState("");
  const [generation, setGeneration] = useState(0);
  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  const { generation: refreshed, track } = useRefresh();
  const [showDone, toggleDone] = useShowDone(userId, "shopping");
  const suggestions = useSuggestions(householdId, "shopping", generation + refreshed);
  const path = { params: { path: { householdId } } };

  useEffect(() => {
    let current = true;
    const request = api
      .GET("/v1/households/{householdId}/shopping-list", { params: { path: { householdId } } })
      .then((res) => current && (res.data ? setItems(res.data) : setFailed(true)))
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

  async function add(e?: FormEvent, picked?: string) {
    e?.preventDefault();
    const body = { text: picked ?? text };
    if (!body.text.trim()) return;
    if (await run(api.POST("/v1/households/{householdId}/shopping-list/items", { ...path, body }))) setText("");
  }

  const item = (id: string) => ({ params: { path: { householdId, itemId: id } } });
  const tick = (it: ShoppingItem) =>
    run(it.done ? api.DELETE("/v1/households/{householdId}/shopping-list/items/{itemId}/done", item(it.id)) : api.PUT("/v1/households/{householdId}/shopping-list/items/{itemId}/done", item(it.id)));

  const open = items?.filter((i) => !i.done) ?? [];
  const done = items?.filter((i) => i.done) ?? [];
  // What's open on the list isn't suggested, even before the suggestions are read again.
  const onList = new Set(open.map((i) => i.text.toLowerCase()));
  const picks = matching(suggestions.filter((s) => !onList.has(s.toLowerCase())), text);
  const row = (it: ShoppingItem) => (
    <div className="entry-row" key={it.id}>
      <button type="button" className={it.done ? "tick tick-done" : "tick"} role="checkbox" aria-checked={it.done} aria-label={it.text} onClick={() => tick(it)}>
        {it.done && <Icon name="check" size="sm" />}
      </button>
      <ListRow
        title={<span className={it.done ? "have" : undefined}>{it.text}</span>}
        subtitle={it.recipe}
        trailing={
          <button type="button" className="screen-action" aria-label={t("shopping.remove", { item: it.text })} onClick={() => run(api.DELETE("/v1/households/{householdId}/shopping-list/items/{itemId}", item(it.id)))}>
            <Icon name="close" size="sm" />
          </button>
        }
      />
    </div>
  );

  return (
    <>
      <form className="shopping-add" onSubmit={add}>
        <input className="input" value={text} onChange={(e) => setText(e.target.value)} placeholder={t("shopping.placeholder")} aria-label={t("shopping.placeholder")} maxLength={200} enterKeyHint="done" />
        <Button type="submit" icon="plus" disabled={!text.trim()} aria-label={t("shopping.add")} />
      </form>
      <Suggestions items={picks} onPick={(s) => add(undefined, s)} />
      {failed && (
        <p className="callout error" role="alert">
          {t("entries.failed")}
        </p>
      )}
      {items?.length === 0 && (
        <Empty title={t("shopping.emptyTitle")}>{t("shopping.emptyBody")}</Empty>
      )}
      {open.length > 0 && <div className="hl-list">{open.map(row)}</div>}
      {done.length > 0 && (
        <section className="group">
          <div className="shopping-done">
            <h2 className="label group-title">{t("entries.done")}</h2>
            <DoneToggle count={done.length} shown={showDone} onToggle={toggleDone} />
            {showDone && (
              <Button variant="plain" size="sm" onClick={() => run(api.DELETE("/v1/households/{householdId}/shopping-list/done", path))}>
                {t("shopping.clear")}
              </Button>
            )}
          </div>
          {showDone && <div className="hl-list">{done.map(row)}</div>}
        </section>
      )}
    </>
  );
}
