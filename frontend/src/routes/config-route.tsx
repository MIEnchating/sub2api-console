import { ConfigPage } from "@/App";
import { useAppShellContext } from "@/app-shell-context";

export function ConfigRoute() {
  const shell = useAppShellContext();
  return (
    <ConfigPage
      hiddenNavigationItemIDs={shell.hiddenNavigationItemIDs}
      onNavigationItemVisibilityChange={shell.setNavigationItemVisibility}
      onResetNavigation={shell.resetNavigation}
    />
  );
}
