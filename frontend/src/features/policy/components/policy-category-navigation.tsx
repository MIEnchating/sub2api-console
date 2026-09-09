import { HeartPulse, Radar, SlidersHorizontal, ScanEye } from "lucide-react";
import type { ReactElement } from "react";

import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { cn } from "@/lib/utils";

const categories = [
  {
    value: "routing",
    label: "调度与写入",
    description: "默认策略、权重与执行",
    icon: SlidersHorizontal,
  },
  { value: "health", label: "健康与处置", description: "评分、熔断与恢复", icon: HeartPulse },
  { value: "sampling", label: "巡检与采样", description: "任务周期与探活样本", icon: Radar },
  { value: "scope", label: "守护范围", description: "管理对象与控制权", icon: ScanEye },
] as const;

export type PolicyCategory = (typeof categories)[number]["value"];

export function PolicyCategoryNavigation(props: {
  value: PolicyCategory;
  onChange: (value: PolicyCategory) => void;
}): ReactElement {
  return (
    <div className="min-w-0" data-testid="policy-category-navigation">
      <SegmentedControl
        role="tablist"
        aria-label="策略分类"
        className="grid w-full grid-cols-2 gap-1 rounded-xl p-1.5 sm:grid-cols-4"
      >
        {categories.map((category) => (
          <SegmentedControlItem
            key={category.value}
            id={`policy-tab-${category.value}`}
            role="tab"
            aria-label={category.label}
            aria-controls={`policy-panel-${category.value}`}
            selected={props.value === category.value}
            onClick={() => props.onChange(category.value)}
            className={cn(
              "h-auto min-h-11 justify-start gap-2 rounded-lg px-2.5 py-2 sm:gap-3 sm:px-3",
              props.value === category.value && "text-primary",
            )}
          >
            <category.icon className="size-4 shrink-0" aria-hidden="true" />
            <span className="min-w-0 text-left">
              <span className="block text-sm font-medium">{category.label}</span>
              <span className="text-muted-foreground mt-0.5 hidden text-xs font-normal xl:block">
                {category.description}
              </span>
            </span>
          </SegmentedControlItem>
        ))}
      </SegmentedControl>
    </div>
  );
}
