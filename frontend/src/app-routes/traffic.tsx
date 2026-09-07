import { createFileRoute } from "@tanstack/react-router";

import { TrafficRankingPage } from "@/features/traffic-ranking/components/traffic-ranking-page";

export const Route = createFileRoute("/traffic")({ component: TrafficRankingPage });
