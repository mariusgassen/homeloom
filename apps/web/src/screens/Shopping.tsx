import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api, type ShoppingItem } from "../api/client";
import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Icon } from "../ui/Icon";
import { ListRow } from "../ui/ListRow";

// The household's shared shopping list: add at the top, tick off, clear what's ticked.
export function Shopping({ householdId }: { householdId: string }) {
  const { t } = useTranslation();
  const [items, setItems] = useState<ShoppingItem[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [text, setText] = useState("");
  const [generation, setGeneration] = useState(0);
  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  const path = { params: { path: { householdId } } };

  useEffect(() => {
    let current = true;
    api
      .GET("/v1/households/{householdId}/shopping-list", { params: { path: { householdId } } })
      .then((res) => current && (res.data ? setItems(res.data) : setFailed(true)))
      .catch(() => current && setFailed(true));
    return () => {
      current = false;
    };
  }, [householdId, generation]);

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
    if (await run(api.POST("/v1/households/{householdId}/shopping-list/items", { ...path, body: { text } }))) setText("");
  }

  const item = (id: string) => ({ params: { path: { householdId, itemId: id } } });
  const tick = (it: ShoppingItem) =>
    run(it.done ? api.DELETE("/v1/households/{householdId}/shopping-list/items/{itemId}/done", item(it.id)) : api.PUT("/v1/households/{householdId}/shopping-list/items/{itemId}/done", item(it.id)));

  const open = items?.filter((i) => !i.done) ?? [];
  const done = items?.filter((i) => i.done) ?? [];
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
      {failed && (
        <p className="callout error" role="alert">
          {t("entries.failed")}
        </p>
      )}
      {items?.length === 0 && (
        <Card tone="sunken" title={t("shopping.emptyTitle")}>
          {t("shopping.emptyBody")}
        </Card>
      )}
      {open.length > 0 && <div className="hl-list">{open.map(row)}</div>}
      {done.length > 0 && (
        <section className="group">
          <div className="shopping-done">
            <h2 className="label group-title">{t("entries.done")}</h2>
            <Button variant="plain" size="sm" onClick={() => run(api.DELETE("/v1/households/{householdId}/shopping-list/done", path))}>
              {t("shopping.clear")}
            </Button>
          </div>
          <div className="hl-list">{done.map(row)}</div>
        </section>
      )}
    </>
  );
}
