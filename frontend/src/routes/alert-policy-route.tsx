import { useNavigate } from "@tanstack/react-router";

import { AlertPolicyPage } from "@/features/alert-policy/components/alert-policy-page";

export function AlertPolicyRoute() {
  const navigate = useNavigate();
  return <AlertPolicyPage onOpenSettings={() => void navigate({ to: "/config" })} />;
}
