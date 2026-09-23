import type { ReactElement } from "react";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { workbenchTabs, type WorkbenchTab } from "../constants";

export function WorkbenchNavigation(props: {
  tab: WorkbenchTab;
  onChange: (tab: WorkbenchTab) => void;
}): ReactElement {
  return (
    <SegmentedControl
      role="tablist"
      aria-label="账号工作台功能"
      className="grid w-full grid-cols-3 sm:inline-flex sm:w-fit"
    >
      {workbenchTabs.map((item) => (
        <SegmentedControlItem
          key={item.id}
          id={`workbench-${item.id}`}
          role="tab"
          selected={props.tab === item.id}
          aria-controls={`workbench-panel-${item.id}`}
          onClick={() => props.onChange(item.id)}
        >
          {item.label}
        </SegmentedControlItem>
      ))}
    </SegmentedControl>
  );
}
