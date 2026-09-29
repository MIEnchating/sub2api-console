import { createFileRoute } from "@tanstack/react-router";

import { historyNavigation } from "@/features/upstreams/lib/history-navigation";

import { UpstreamsPage } from "@/App";

export const Route = createFileRoute("/upstreams")({
  component: UpstreamsPage,
  validateSearch: historyNavigation,
});
