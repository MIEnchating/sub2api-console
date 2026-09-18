import { lazy, Suspense } from "react";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";

const NewAPISettings = lazy(() =>
  import("@/features/newapi-management/components/newapi-management-page").then((module) => ({
    default: module.NewAPIManagementPage,
  })),
);
const MonitoringSettings = lazy(() =>
  import("@/features/uptime-kuma/components/kuma-config-page").then((module) => ({
    default: module.KumaConfigPage,
  })),
);

export function PlatformSettingsPage(props: { activeTab: "newapi" | "monitoring" }) {
  return (
    <div
      id={`config-panel-${props.activeTab}`}
      role="tabpanel"
      aria-labelledby={`config-tab-${props.activeTab}`}
      className="h-full min-h-0 min-w-0"
    >
      <Suspense
        fallback={<PageLoadingSkeleton label="正在读取平台设置" variant="form" panels={2} />}
      >
        {props.activeTab === "newapi" ? (
          <NewAPISettings view="platform" embedded />
        ) : (
          <MonitoringSettings embedded />
        )}
      </Suspense>
    </div>
  );
}
