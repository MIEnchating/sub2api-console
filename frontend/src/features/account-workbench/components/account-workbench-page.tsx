import { useState, type ReactElement } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { SetupStatus } from "@/api";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { workbenchTabs, type WorkbenchTab } from "../constants";
import { WorkbenchImport } from "./workbench-import";
import { WorkbenchMixed } from "./workbench-mixed";
import { WorkbenchTemplates } from "./workbench-templates";
import { WorkbenchHistory } from "./workbench-history";
import { WorkbenchMaintenancePanel } from "./workbench-maintenance";
import { WorkbenchOAuthPanel } from "./workbench-oauth-panel";
import { WorkbenchExportPanel } from "./workbench-export-panel";
import { WorkbenchSecurityPanel } from "./workbench-security-panel";
import { WorkbenchLocalExport } from "./workbench-local-export";

export function AccountWorkbenchPage(): ReactElement {
  const client = useQueryClient();
  const [tab, setTab] = useState<WorkbenchTab>(() =>
    client.getQueryData<SetupStatus>(["setup-status"])?.target_configured === false
      ? "local"
      : "import",
  );
  return (
    <PageLayout
      navigation={
        <SegmentedControl role="tablist" aria-label="账号工作台功能">
          {workbenchTabs.map((item) => (
            <SegmentedControlItem
              key={item.id}
              id={`workbench-tab-${item.id}`}
              role="tab"
              selected={tab === item.id}
              aria-controls={`workbench-panel-${item.id}`}
              onClick={() => setTab(item.id)}
            >
              {item.label}
            </SegmentedControlItem>
          ))}
        </SegmentedControl>
      }
    >
      <PageHeading
        eyebrow="OpenAI OAuth"
        title="账号工作台"
        description="批量导入账号、复用配置模板，并维护已有账号的登录状态。"
      />
      <section
        id={`workbench-panel-${tab}`}
        role="tabpanel"
        aria-labelledby={`workbench-tab-${tab}`}
        className="min-w-0"
      >
        {tab === "import" && <WorkbenchImport />}
        {tab === "mixed" && <WorkbenchMixed />}
        {tab === "oauth" && <WorkbenchOAuthPanel />}
        {tab === "templates" && <WorkbenchTemplates />}
        {tab === "exports" && <WorkbenchExportPanel />}
        {tab === "local" && <WorkbenchLocalExport />}
        {tab === "security" && <WorkbenchSecurityPanel />}
        {tab === "history" && <WorkbenchHistory />}
        {tab === "maintenance" && <WorkbenchMaintenancePanel />}
      </section>
    </PageLayout>
  );
}
