import { createFileRoute } from "@tanstack/react-router";

import { AccountModelAutomationPage } from "@/features/newapi-management/components/account-model-automation-page";

export const Route = createFileRoute("/newapi/account-model-automation")({
  component: AccountModelAutomationPage,
});
