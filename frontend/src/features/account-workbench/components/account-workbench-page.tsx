import { useState, type ReactElement } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ClipboardList, FileInput, SlidersHorizontal, Wrench } from "lucide-react";
import type { SetupStatus } from "@/api";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { workbenchPrimaryTabs, type WorkbenchPrimaryTab } from "../constants";
import { WorkbenchMixed } from "./workbench-mixed";
import { WorkbenchTemplates } from "./workbench-templates";
import { WorkbenchHistory } from "./workbench-history";
import { WorkbenchMaintenancePanel } from "./workbench-maintenance";
import { WorkbenchCheckerStatus, WorkbenchRecentRun } from "./workbench-import-status";

const primaryIcons = {
  accounts: FileInput,
  templates: SlidersHorizontal,
  history: ClipboardList,
  maintenance: Wrench,
};

export function AccountWorkbenchPage(): ReactElement {
  const client = useQueryClient();
  const [tab, setTab] = useState<WorkbenchPrimaryTab>("accounts");
  const [activeTask, setActiveTask] = useState<string | null>(null);
  const local = client.getQueryData<SetupStatus>(["setup-status"])?.target_configured === false;
  return (
    <PageLayout
      navigation={
        <SegmentedControl
          role="tablist"
          aria-label="账号工作台功能"
          className="grid w-full grid-cols-2 gap-1 rounded-none border-0 border-b bg-transparent p-0 sm:flex sm:flex-wrap"
        >
          {workbenchPrimaryTabs.map((item) => {
            const Icon = primaryIcons[item.id];
            return (
              <SegmentedControlItem
                key={item.id}
                id={`workbench-primary-${item.id}`}
                role="tab"
                selected={tab === item.id}
                aria-controls={`workbench-primary-panel-${item.id}`}
                onClick={() => setTab(item.id)}
                className="min-w-0 rounded-none border-0 border-b-2 border-transparent bg-transparent px-3 shadow-none aria-selected:border-primary aria-selected:text-primary sm:min-w-max"
              >
                <Icon aria-hidden="true" />
                {item.label}
              </SegmentedControlItem>
            );
          })}
        </SegmentedControl>
      }
    >
      <PageHeading eyebrow="" title="账号工作台" description="" />
      {/* Keep the current input and authorization alive while switching workbench tabs. */}
      <section
        id="workbench-primary-panel-accounts"
        role="tabpanel"
        aria-labelledby="workbench-primary-accounts"
        hidden={tab !== "accounts"}
        className="min-w-0"
      >
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-sm font-medium">添加账号</h2>
          <WorkbenchCheckerStatus />
        </div>
        <WorkbenchMixed
          scope={local ? "local-export" : undefined}
          onActiveTaskChange={setActiveTask}
        />
        <WorkbenchRecentRun onOpen={() => setTab("history")} />
      </section>
      {tab !== "accounts" && (
        <section
          id={`workbench-primary-panel-${tab}`}
          role="tabpanel"
          aria-labelledby={`workbench-primary-${tab}`}
          className="min-w-0"
        >
          {tab === "templates" &&
            (local ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                连接管理站点后可读取账号配置模板
              </p>
            ) : (
              <WorkbenchTemplates />
            ))}
          {tab === "history" && (
            <WorkbenchHistory activeTaskId={activeTask} onContinue={() => setTab("accounts")} />
          )}
          {tab === "maintenance" &&
            (local ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                连接管理站点后可配置自动维护
              </p>
            ) : (
              <WorkbenchMaintenancePanel />
            ))}
        </section>
      )}
    </PageLayout>
  );
}
