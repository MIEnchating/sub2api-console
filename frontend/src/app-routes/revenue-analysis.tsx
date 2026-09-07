import { createFileRoute } from "@tanstack/react-router";

import { RevenueAnalysisRoute } from "@/routes/pricing-routes";

export const Route = createFileRoute("/revenue-analysis")({ component: RevenueAnalysisRoute });
