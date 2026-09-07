import { createFileRoute } from "@tanstack/react-router";

import { PricingRoute } from "@/routes/pricing-routes";

export const Route = createFileRoute("/pricing")({ component: PricingRoute });
