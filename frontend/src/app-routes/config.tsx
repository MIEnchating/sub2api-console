import { createFileRoute } from "@tanstack/react-router";

import { normalizeConfigTab, type ConfigTab } from "@/features/config/constants";
import { ConfigRoute } from "@/routes/config-route";

export const Route = createFileRoute("/config")({
  component: ConfigRoute,
  validateSearch: (search: Record<string, unknown>): { tab?: ConfigTab } => {
    const tab = normalizeConfigTab(search.tab);
    return tab === "connection" ? {} : { tab };
  },
});
