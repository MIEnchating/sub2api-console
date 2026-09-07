import { createFileRoute } from "@tanstack/react-router";

import { OverviewRoute } from "@/routes/overview-route";

export const Route = createFileRoute("/")({ component: OverviewRoute });
