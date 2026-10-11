import type { ReactElement } from "react";
import { FileInput, HeartPulse, History, LayoutTemplate, Users } from "lucide-react";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { cn } from "@/lib/utils";
import { workbenchTabs, type WorkbenchTab } from "../constants";

const tabIcons = {
  import: FileInput,
  accounts: Users,
  templates: LayoutTemplate,
  records: History,
  maintenance: HeartPulse,
};

export function WorkbenchNavigation(props: {
  tab: WorkbenchTab;
  onChange: (tab: WorkbenchTab) => void;
}): ReactElement {
  return (
    <SegmentedControl
      role="tablist"
      aria-label="账号工作台功能"
      className="grid w-full grid-cols-3 rounded-lg bg-card p-1.5 sm:inline-flex sm:w-fit"
    >
      {workbenchTabs.map((item) => {
        const Icon = tabIcons[item.id];
        return (
          <SegmentedControlItem
            key={item.id}
            id={`workbench-${item.id}`}
            role="tab"
            selected={props.tab === item.id}
            aria-controls={`workbench-panel-${item.id}`}
            onClick={() => props.onChange(item.id)}
            className={cn(
              "min-w-0 px-2 sm:px-3",
              props.tab === item.id && "bg-primary/10 text-primary shadow-none hover:bg-primary/15",
            )}
          >
            <Icon aria-hidden="true" className="hidden size-4 sm:block" />
            {item.label}
          </SegmentedControlItem>
        );
      })}
    </SegmentedControl>
  );
}
