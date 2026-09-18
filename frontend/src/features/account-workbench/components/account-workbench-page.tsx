import { useState, type ReactElement } from "react";
import { PageLayout } from "@/components/page-layout";
import { PageHeading } from "@/components/page-heading";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { workbenchTabs } from "../constants";
import { cn } from "@/lib/utils";
import { AccountList } from "./account-list";
import { TemplateList } from "./template-list";
import { ImportPanel } from "./import-panel";
import { RunList } from "./run-list";
import { MaintenancePanel } from "./maintenance-panel";

export function AccountWorkbenchPage(): ReactElement {
  const [tab, setTab] = useState<(typeof workbenchTabs)[number]["id"]>("import");
  return (
    <PageLayout
      navigation={
        <SegmentedControl role="tablist" aria-label="账号工作台功能" className="w-full sm:w-fit">
          {workbenchTabs.map((item) => (
            <SegmentedControlItem
              key={item.id}
              id={`workbench-${item.id}`}
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
      <PageHeading eyebrow="" title="账号工作台" description="" />
      <section
        role="tabpanel"
        id={`workbench-panel-${tab}`}
        aria-labelledby={`workbench-${tab}`}
        className={cn(
          "min-w-0",
          tab === "import" ? "h-full min-h-0" : "rounded-xl border bg-card p-3 sm:p-4",
        )}
      >
        {tab === "accounts" && <AccountList />}
        {tab === "templates" && <TemplateList />}
        {tab === "import" && <ImportPanel onStarted={() => setTab("records")} />}
        {tab === "records" && <RunList />}
        {tab === "maintenance" && <MaintenancePanel />}
      </section>
    </PageLayout>
  );
}
