import { useTranslation } from "react-i18next";
import { useEffect, useMemo, useState } from "react";
import { matchPath, Navigate, Route, Routes, useLocation } from "react-router";
import { CallProvider } from "./call";
import { CallView, GuestCall, IncomingCall } from "./screens/Call";
import { AssistantContext } from "./assistant";
import { Assistant } from "./screens/Assistant";
import { useNotificationLinks } from "./links";
import { Join, pendingInvite } from "./screens/Join";
import { Onboarding } from "./screens/Onboarding";
import { Settings } from "./screens/Settings";
import { RefreshProvider } from "./refresh";
import { Chores } from "./screens/Chores";
import { Cook, RecipeScreen } from "./screens/Cook";
import { Home } from "./screens/Home";
import { Lists } from "./screens/Lists";
import { NotePage } from "./screens/NotePage";
import { Today, Us } from "./screens/Tabs";
import { Watch } from "./screens/Watch";
import { Loading, Unavailable, Welcome } from "./screens/Welcome";
import { useSession } from "./session";
import { TabBar } from "./ui/TabBar";

export function App() {
  const { t } = useTranslation();
  const { session, reload } = useSession();
  const { pathname } = useLocation();
  const linked = matchPath("/join/:code", pathname)?.params.code;
  const guest = matchPath("/guest/:token", pathname)?.params.token;
  const invite = linked ?? pendingInvite.get();
  const [asking, setAsking] = useState(false);
  // Bumped when the assistant may have changed something, so the screen behind it loads again.
  const [generation, setGeneration] = useState(0);
  const assistant = useMemo(() => ({ open: () => setAsking(true) }), []);
  useNotificationLinks();

  useEffect(() => {
    if (linked) pendingInvite.set(linked);
  }, [linked]);

  // A guest link works for anyone, signed in or not.
  if (guest) {
    return (
      <CallProvider>
        <GuestCall token={guest} />
      </CallProvider>
    );
  }

  switch (session.state) {
    case "loading":
      return <Loading />;
    case "signedOut":
      return <Welcome invited={!!invite} />;
    case "unavailable":
      return <Unavailable onRetry={reload} />;
  }
  if (invite) return <Join code={invite} />;
  if (!session.household) return <Onboarding />;

  return (
    <AssistantContext.Provider value={assistant}>
      <CallProvider>
        <div className="app">
          <RefreshProvider>
            <Routes key={generation}>
              <Route path="/" element={<Today />} />
              <Route path="/cook" element={<Cook />} />
              <Route path="/cook/:slug" element={<RecipeScreen />} />
              <Route path="/us" element={<Us />} />
              <Route path="/watch" element={<Watch />} />
              <Route path="/lists" element={<Lists />} />
              <Route path="/lists/notes/:id" element={<NotePage />} />
              <Route path="/chores" element={<Chores />} />
              <Route path="/home" element={<Home />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="/call/:callId" element={<IncomingCall />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </RefreshProvider>
          <TabBar
            label={t("tabs.label")}
            items={[
              { to: "/", label: t("tabs.today"), icon: "today" },
              { to: "/us", label: t("tabs.us"), icon: "us" },
              { to: "/lists", label: t("tabs.lists"), icon: "lists" },
              { to: "/home", label: t("tabs.home"), icon: "home" },
            ]}
          />
          {asking && (
            <Assistant
              householdId={session.household.id}
              onClose={(changed) => {
                setAsking(false);
                if (changed) setGeneration((g) => g + 1);
              }}
            />
          )}
          <CallView />
        </div>
      </CallProvider>
    </AssistantContext.Provider>
  );
}
