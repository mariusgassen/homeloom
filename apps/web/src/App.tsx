import { useTranslation } from "react-i18next";
import { useEffect } from "react";
import { matchPath, Navigate, Route, Routes, useLocation } from "react-router";
import { CallProvider } from "./call";
import { CallView, IncomingCall } from "./screens/Call";
import { Join, pendingInvite } from "./screens/Join";
import { Onboarding } from "./screens/Onboarding";
import { Settings } from "./screens/Settings";
import { Cook, RecipeScreen } from "./screens/Cook";
import { Home } from "./screens/Home";
import { Lists } from "./screens/Lists";
import { Today, Us } from "./screens/Tabs";
import { Unavailable, Welcome } from "./screens/Welcome";
import { useSession } from "./session";
import { TabBar } from "./ui/TabBar";

export function App() {
  const { t } = useTranslation();
  const { session, reload } = useSession();
  const linked = matchPath("/join/:code", useLocation().pathname)?.params.code;
  const invite = linked ?? pendingInvite.get();

  useEffect(() => {
    if (linked) pendingInvite.set(linked);
  }, [linked]);

  switch (session.state) {
    case "loading":
      return null;
    case "signedOut":
      return <Welcome invited={!!invite} />;
    case "unavailable":
      return <Unavailable onRetry={reload} />;
  }
  if (invite) return <Join code={invite} />;
  if (!session.household) return <Onboarding />;

  return (
    <CallProvider>
      <div className="app">
        <Routes>
          <Route path="/" element={<Today />} />
          <Route path="/cook" element={<Cook />} />
          <Route path="/cook/:slug" element={<RecipeScreen />} />
          <Route path="/us" element={<Us />} />
          <Route path="/lists" element={<Lists />} />
          <Route path="/home" element={<Home />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="/call" element={<IncomingCall />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
        <TabBar
          label={t("tabs.label")}
          items={[
            { to: "/", label: t("tabs.today"), icon: "today" },
            { to: "/cook", label: t("tabs.cook"), icon: "cook" },
            { to: "/us", label: t("tabs.us"), icon: "us" },
            { to: "/lists", label: t("tabs.lists"), icon: "lists" },
            { to: "/home", label: t("tabs.home"), icon: "home" },
          ]}
        />
        <CallView />
      </div>
    </CallProvider>
  );
}
