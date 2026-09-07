import { createFileRoute } from "@tanstack/react-router";

import { SystemInfoPage } from "@/features/system-info/components/system-info-page";

export const Route = createFileRoute("/system-info")({ component: SystemInfoPage });
