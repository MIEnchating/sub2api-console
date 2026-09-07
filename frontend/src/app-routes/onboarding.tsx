import { createFileRoute } from "@tanstack/react-router";

import { OnboardingPage } from "@/App";

export const Route = createFileRoute("/onboarding")({
  component: OnboardingPage,
  validateSearch: (search: Record<string, unknown>) => ({
    host: typeof search.host === "string" ? search.host : undefined,
    upstream_type: typeof search.upstream_type === "string" ? search.upstream_type : undefined,
    group_id: typeof search.group_id === "string" ? search.group_id : undefined,
  }),
});
