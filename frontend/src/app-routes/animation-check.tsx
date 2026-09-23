import { createFileRoute } from "@tanstack/react-router";
import { AnimationCheckPage } from "@/features/model-check/components/animation-check-page";
import { modelCheckSearch } from "@/features/model-check/lib/account-link";

export const Route = createFileRoute("/animation-check")({
  validateSearch: modelCheckSearch,
  component: LinkedAnimationCheckPage,
});

function LinkedAnimationCheckPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <AnimationCheckPage
      key={search.account_id ?? "all"}
      accountID={search.account_id}
      onBackToAccounts={() => void navigate({ to: "/accounts" })}
    />
  );
}
