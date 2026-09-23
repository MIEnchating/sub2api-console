import { createFileRoute } from "@tanstack/react-router";
import { AccountWorkbenchPage } from "@/features/account-workbench/components/account-workbench-page";

export const Route = createFileRoute("/account-workbench/")({ component: WorkbenchIndex });
function WorkbenchIndex() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <AccountWorkbenchPage
      tab={search.tab ?? "import"}
      onStarted={() => void navigate({ search: { tab: "records" } })}
    />
  );
}
