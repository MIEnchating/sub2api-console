import { createFileRoute } from "@tanstack/react-router";
import { KumaTemplatesPage } from "@/features/uptime-kuma/components/templates-page";
export const Route = createFileRoute("/uptime-kuma/templates")({ component: KumaTemplatesPage });
