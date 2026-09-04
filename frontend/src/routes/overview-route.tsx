import { useNavigate } from "@tanstack/react-router";

import { OverviewPage } from "@/features/overview/components/overview-page";

export function OverviewRoute() {
  const navigate = useNavigate();
  return (
    <OverviewPage
      onOpenAccounts={() => void navigate({ to: "/accounts" })}
      onOpenEvents={() => void navigate({ to: "/logs", search: { kind: "event" } })}
      onOpenGroups={() => void navigate({ to: "/groups" })}
    />
  );
}
