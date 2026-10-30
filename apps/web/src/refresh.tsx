import { createContext, use, useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import { drain, onSynced } from "./outbox";

// A screen's data hooks list generation in their effect deps and pass their request to track, so
// a pull (or coming back to the app) loads them again and the spinner knows when they're done.
type Value = { generation: number; track: (request: Promise<unknown>) => void };

const RefreshContext = createContext<Value>({ generation: 0, track: () => undefined });

export function useRefresh() {
  return use(RefreshContext);
}

// The pull spinner shows at least this long, so a quick answer still reads as "loaded".
const minSpin = 600;
// Coming back to the app after this long loads the screen again without a pull.
const staleAfter = 60_000;

export function RefreshProvider({ children }: { children: ReactNode }) {
  const [generation, setGeneration] = useState(0);
  const pending = useRef(new Set<Promise<unknown>>());
  const online = useOnline();

  const track = useCallback((request: Promise<unknown>) => {
    pending.current.add(request);
    const done = () => pending.current.delete(request);
    request.then(done, done);
  }, []);

  const refresh = useCallback(async () => {
    setGeneration((g) => g + 1);
    // Let the screen render and start its requests before waiting on them.
    await new Promise((r) => setTimeout(r, 50));
    await Promise.allSettled([...pending.current, new Promise((r) => setTimeout(r, minSpin))]);
  }, []);

  // Changes made offline go out when the app opens, comes back or is online again; once they
  // have, the screen loads again to show what the server made of them.
  useEffect(() => onSynced(() => setGeneration((g) => g + 1)), []);
  useEffect(() => {
    if (online) void drain();
  }, [online]);

  // Back online, or back in the app after a while: load again quietly.
  const wasOnline = useRef(online);
  useEffect(() => {
    if (online && !wasOnline.current) setGeneration((g) => g + 1);
    wasOnline.current = online;
  }, [online]);
  useEffect(() => {
    let hiddenAt = 0;
    const onVisibility = () => {
      if (document.hidden) hiddenAt = Date.now();
      else void drain();
      if (!document.hidden && hiddenAt && Date.now() - hiddenAt > staleAfter) setGeneration((g) => g + 1);
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, []);

  const value = useMemo(() => ({ generation, track }), [generation, track]);
  return (
    <RefreshContext value={value}>
      <PullContext value={refresh}>{children}</PullContext>
    </RefreshContext>
  );
}

const PullContext = createContext<() => Promise<void>>(async () => undefined);

export function usePull() {
  return use(PullContext);
}

function subscribe(onChange: () => void) {
  window.addEventListener("online", onChange);
  window.addEventListener("offline", onChange);
  return () => {
    window.removeEventListener("online", onChange);
    window.removeEventListener("offline", onChange);
  };
}

export function useOnline() {
  return useSyncExternalStore(
    subscribe,
    () => navigator.onLine,
    () => true,
  );
}
