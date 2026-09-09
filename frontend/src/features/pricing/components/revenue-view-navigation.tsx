import type { ReactElement } from "react";

import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";

const revenueViews = [
  { id: "details", label: "账号明细" },
  { id: "summary", label: "金额统计" },
  { id: "issues", label: "上游读取问题" },
] as const;

export type RevenueAnalysisView = (typeof revenueViews)[number]["id"];

export function RevenueViewNavigation(props: {
  value: RevenueAnalysisView;
  onChange: (value: RevenueAnalysisView) => void;
}): ReactElement {
  return (
    <SegmentedControl
      role="tablist"
      aria-label="收益分析视图"
      className="grid w-full grid-cols-3 sm:w-fit"
    >
      {revenueViews.map((view) => (
        <SegmentedControlItem
          key={view.id}
          id={`revenue-tab-${view.id}`}
          role="tab"
          aria-controls={`revenue-panel-${view.id}`}
          selected={props.value === view.id}
          onClick={() => props.onChange(view.id)}
          className="h-9 min-w-0 px-2 sm:px-4"
        >
          {view.label}
        </SegmentedControlItem>
      ))}
    </SegmentedControl>
  );
}
