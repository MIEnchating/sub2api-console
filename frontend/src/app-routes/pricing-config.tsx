import { createFileRoute } from "@tanstack/react-router";

import { PricingConfigRoute } from "@/routes/pricing-routes";

export const Route = createFileRoute("/pricing-config")({ component: PricingConfigRoute });
