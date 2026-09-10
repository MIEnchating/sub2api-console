import { createFileRoute } from "@tanstack/react-router";
import { KumaResourcesPage } from "@/features/uptime-kuma/components/resources-page";
export const Route = createFileRoute("/uptime-kuma/status-pages")({ component: Page });
function Page() {
  return <KumaResourcesPage kind="status-pages" />;
}
