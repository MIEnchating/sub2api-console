import { createFileRoute } from "@tanstack/react-router";

import { AutoInspectionPage } from "@/App";

export const Route = createFileRoute("/auto-inspection")({ component: AutoInspectionPage });
