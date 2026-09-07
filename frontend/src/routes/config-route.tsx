import { ConfigPage } from "@/App";
import { useAppShellContext } from "@/app-shell-context";
import { normalizeConfigTab, type ConfigTab } from "@/features/config/constants";
import { useNavigate, useSearch } from "@tanstack/react-router";

export function ConfigRoute() {
  const shell = useAppShellContext();
  const search = useSearch({ strict: false }) as { tab?: unknown };
  const navigate = useNavigate();
  const activeTab = normalizeConfigTab(search.tab);

  function selectTab(tab: ConfigTab) {
    void navigate({ to: "/config", search: { tab }, replace: true });
  }

  return (
    <ConfigPage
      activeTab={activeTab}
      onTabChange={selectTab}
      hiddenNavigationItemIDs={shell.hiddenNavigationItemIDs}
      onNavigationItemVisibilityChange={shell.setNavigationItemVisibility}
      onResetNavigation={shell.resetNavigation}
    />
  );
}
