import type { ReactElement } from "react";
import { Outlet, useNavigate, useSearch } from "@tanstack/react-router";
import { PageLayout } from "@/components/page-layout";
import { cn } from "@/lib/utils";
import { WorkbenchNavigation } from "./workbench-navigation";
import type { WorkbenchTab } from "../constants";

export function WorkbenchLayout(): ReactElement {
  const search = useSearch({ from: "/account-workbench" });
  const tab = search.tab ?? "import";
  const navigate = useNavigate();
  const changeTab = (next: WorkbenchTab): void => {
    void navigate({ to: "/account-workbench", search: { tab: next } });
  };
  return (
    <PageLayout navigation={<WorkbenchNavigation tab={tab} onChange={changeTab} />}>
      <section
        role="tabpanel"
        id={`workbench-panel-${tab}`}
        aria-labelledby={`workbench-${tab}`}
        className={cn("min-w-0", tab === "import" && "h-full min-h-0")}
      >
        <Outlet />
      </section>
    </PageLayout>
  );
}
