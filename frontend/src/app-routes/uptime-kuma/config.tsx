import { createFileRoute } from "@tanstack/react-router";
import { KumaConfigPage } from "@/features/uptime-kuma/components/kuma-config-page";
export const Route = createFileRoute("/uptime-kuma/config")({ component: KumaConfigPage });
