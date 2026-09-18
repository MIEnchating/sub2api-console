import { createFileRoute } from "@tanstack/react-router";

import { ModelCheckPage } from "@/features/model-check/components/model-check-page";
import { modelCheckSearch } from "@/features/model-check/lib/account-link";

export const Route = createFileRoute("/model-check")({
  validateSearch: modelCheckSearch,
  component: LinkedModelCheckPage,
});

function LinkedModelCheckPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <ModelCheckPage
      key={search.account_id ?? "all"}
      accountID={search.account_id}
      onBackToAccounts={() => void navigate({ to: "/accounts" })}
    />
  );
}
