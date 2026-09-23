import { createFileRoute } from "@tanstack/react-router";
import { WorkbenchLayout } from "@/features/account-workbench/components/workbench-layout";
import { workbenchTabs, type WorkbenchTab } from "@/features/account-workbench/constants";

export const Route = createFileRoute("/account-workbench")({
  validateSearch: (search: Record<string, unknown>): { tab?: WorkbenchTab } => {
    const tab = workbenchTabs.find((item) => item.id === search.tab)?.id;
    return tab ? { tab } : {};
  },
  component: WorkbenchLayout,
});
