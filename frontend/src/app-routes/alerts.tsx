import { createFileRoute } from "@tanstack/react-router";

import { AlertsPage } from "@/App";

export const Route = createFileRoute("/alerts")({ component: AlertsPage });
