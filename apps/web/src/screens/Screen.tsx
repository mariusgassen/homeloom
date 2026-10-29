import { useRef, useState, type ReactNode, type TouchEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { useAssistant } from "../assistant";
import { useOnline, usePull } from "../refresh";
import { useSession } from "../session";
import { cx, Icon } from "../ui/Icon";
import { Mark } from "../ui/Mark";
import { ThreadAvatar, threadFor } from "../ui/ThreadAvatar";

// A tab's page: the assistant and household buttons top right, a large title, then content. back
// replaces the overline on pages below a tab; action is a button left of those, such as call. add
// is the round plus above the tab bar, in reach of the thumb, for what this screen holds.
// Pulling down from the top loads the screen again, unless refreshable is false.
export function Screen({ title, overline, back, action, add, refreshable = true, children }: {
  title: string;
  overline?: string;
  back?: ReactNode;
  action?: ReactNode;
  add?: { label: string; onClick: () => void };
  refreshable?: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const { session } = useSession();
  const assistant = useAssistant();
  const online = useOnline();
  const pull = usePullToRefresh(refreshable);
  const members = session.state === "signedIn" ? (session.household?.members ?? []) : [];
  return (
    <div className={cx("screen", add && "screen-with-add")} {...pull.handlers}>
      <div className="screen-toolbar">
        {back ?? <span className="caption screen-overline">{overline}</span>}
        {action}
        <button type="button" className="screen-action" aria-label={t("assistant.open")} onClick={assistant.open}>
          <Icon name="sparkle" />
        </button>
        <Link to="/settings" className="screen-household" aria-label={t("settings.open")}>
          <span className="hl-pair">
            {members.map((m, i) => (
              <ThreadAvatar key={m.user.id} name={m.user.name} src={m.user.avatarUrl} thread={threadFor(i)} size="sm" />
            ))}
          </span>
        </Link>
      </div>
      {refreshable && (
        <div className={cx("ptr", !pull.dragging && "ptr-settle")} style={{ height: pull.distance }} aria-hidden={!pull.refreshing}>
          <Mark size={28} progress={pull.refreshing ? 1 : pull.distance / threshold} weaving={pull.refreshing} />
          {pull.refreshing && <span className="visually-hidden" role="status">{t("refresh.loading")}</span>}
        </div>
      )}
      {!online && (
        <p className="caption offline" role="status">
          {t("refresh.offline")}
        </p>
      )}
      <header className="screen-header">
        <h1 className="large-title">{title}</h1>
      </header>
      <main className="screen-main">{children}</main>
      {add && (
        <button type="button" className="screen-add" aria-label={add.label} onClick={add.onClick}>
          <Icon name="plus" />
        </button>
      )}
    </div>
  );
}

// How far to pull before letting go loads again; the content follows the finger at half speed.
const threshold = 64;

// iOS has no pull to refresh in a home-screen app, so the screen follows the finger itself when a
// touch starts at the very top and moves down. Touches in sheets, overlays and fields are theirs.
function usePullToRefresh(enabled: boolean) {
  const refresh = usePull();
  const start = useRef<{ x: number; y: number } | null>(null);
  const [distance, setDistance] = useState(0);
  const [dragging, setDragging] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const onTouchStart = (e: TouchEvent) => {
    const target = e.target as Element;
    if (!enabled || refreshing || window.scrollY > 0 || e.touches.length > 1) return;
    if (target.closest(".sheet-backdrop, .assistant, .call, input, textarea, select")) return;
    const touch = e.touches[0];
    if (touch) start.current = { x: touch.clientX, y: touch.clientY };
  };
  const onTouchMove = (e: TouchEvent) => {
    const touch = e.touches[0];
    if (!start.current || !touch) return;
    const dx = touch.clientX - start.current.x;
    const dy = touch.clientY - start.current.y;
    // A sideways swipe or a scroll up isn't a pull.
    if ((!dragging && Math.abs(dx) > dy) || window.scrollY > 0) {
      start.current = null;
      setDragging(false);
      setDistance(0);
      return;
    }
    setDragging(true);
    setDistance(Math.min(threshold * 1.5, Math.max(0, dy / 2)));
  };
  const onTouchEnd = async () => {
    if (!start.current) return;
    start.current = null;
    setDragging(false);
    if (distance < threshold) {
      setDistance(0);
      return;
    }
    setDistance(threshold);
    setRefreshing(true);
    await refresh();
    setRefreshing(false);
    setDistance(0);
  };
  return { distance, dragging, refreshing, handlers: { onTouchStart, onTouchMove, onTouchEnd, onTouchCancel: onTouchEnd } };
}
