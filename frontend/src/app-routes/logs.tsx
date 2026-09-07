import { createFileRoute } from "@tanstack/react-router";

import { LogsCenterPage } from "@/features/logs/components/logs-center-page";

export const Route = createFileRoute("/logs")({
  component: LogsCenterPage,
  validateSearch: (search: Record<string, unknown>) => ({
    kind:
      typeof search.kind === "string" && ["all", "task", "event", "change"].includes(search.kind)
        ? search.kind
        : "all",
  }),
});
