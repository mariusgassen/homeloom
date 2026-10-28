import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes } from "react-router";
import { Onboarding } from "./screens/Onboarding";
import { Settings } from "./screens/Settings";
import { Cook, Home, Lists, Today, Us } from "./screens/Tabs";
import { Unavailable, Welcome } from "./screens/Welcome";
import { useSession } from "./session";
import { TabBar } from "./ui/TabBar";

export function App() {
  const { t } = useTranslation();
  const { session, reload } = useSession();

  switch (session.state) {
    case "loading":
      return null;
    case "signedOut":
      return <Welcome />;
    case "unavailable":
      return <Unavailable onRetry={reload} />;
  }
  if (!session.household) return <Onboarding />;

  return (
    <div className="app">
      <Routes>
        <Route path="/" element={<Today />} />
        <Route path="/cook" element={<Cook />} />
        <Route path="/us" element={<Us />} />
        <Route path="/lists" element={<Lists />} />
        <Route path="/home" element={<Home />} />
        <Route path="/settings" element={<Settings />} />
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
    </div>
  );
}
