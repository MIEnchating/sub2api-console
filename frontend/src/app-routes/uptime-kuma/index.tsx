import { createFileRoute } from "@tanstack/react-router";
import { UptimeKumaPage } from "@/features/uptime-kuma/components/uptime-kuma-page";
export const Route = createFileRoute("/uptime-kuma/")({ component: UptimeKumaPage });
