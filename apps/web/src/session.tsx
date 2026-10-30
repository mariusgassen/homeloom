import { createContext, use, useCallback, useEffect, useState, type ReactNode } from "react";
import { api, type Household, type HouseholdDetail, type User } from "./api/client";
import { cached, forget } from "./cache";

export type Session =
  | { state: "loading" }
  | { state: "signedOut" }
  | { state: "unavailable" }
  | { state: "signedIn"; user: User; household?: HouseholdDetail };

type Value = { session: Session; reload: () => void; signOut: () => Promise<void> };

const SessionContext = createContext<Value | null>(null);

async function load(): Promise<Session> {
  try {
    const me = await api.GET("/v1/me");
    if (me.response.status === 401) return { state: "signedOut" };
    if (!me.data) return { state: "unavailable" };
    const list = await api.GET("/v1/households");
    if (!list.data) return { state: "unavailable" };
    // One household per couple for now; the first one is home.
    const first = list.data[0];
    if (!first) return { state: "signedIn", user: me.data };
    const detail = await api.GET("/v1/households/{householdId}", { params: { path: { householdId: first.id } } });
    if (!detail.data) return { state: "unavailable" };
    return { state: "signedIn", user: me.data, household: detail.data };
  } catch {
    return { state: "unavailable" };
  }
}

// The session this device kept from last time, so the app opens signed in at once and checks behind it.
function kept(): Session {
  const user = cached<User>("/v1/me");
  const first = cached<Household[]>("/v1/households")?.[0];
  const household = first && cached<HouseholdDetail>(`/v1/households/${first.id}`);
  if (!user || !first || !household) return { state: "loading" };
  return { state: "signedIn", user, household };
}

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session>(kept);
  const [generation, setGeneration] = useState(0);

  useEffect(() => {
    let current = true;
    load().then((s) => current && setSession(s));
    return () => {
      current = false;
    };
  }, [generation]);

  const reload = useCallback(() => setGeneration((g) => g + 1), []);
  const signOut = useCallback(async () => {
    const res = await api.POST("/auth/logout").catch(() => undefined);
    forget();
    const next = res?.data?.redirectUrl;
    if (next) window.location.assign(next);
    else setSession({ state: "signedOut" });
  }, []);

  return <SessionContext value={{ session, reload, signOut }}>{children}</SessionContext>;
}

export function useSession() {
  const v = use(SessionContext);
  if (!v) throw new Error("useSession outside SessionProvider");
  return v;
}
