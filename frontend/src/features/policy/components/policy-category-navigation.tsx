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
        className="bg-muted/30 grid w-full grid-cols-2 items-stretch gap-1.5 rounded-xl p-1.5 sm:grid-cols-4"
      >
        {categories.map((category) => (
          <SegmentedControlItem
            key={category.value}
            id={`policy-tab-${category.value}`}
            role="tab"
            aria-label={category.label}
            aria-controls={`policy-panel-${category.value}`}
            aria-describedby={`policy-category-description-${category.value}`}
            selected={props.value === category.value}
            onClick={() => props.onChange(category.value)}
            className={cn(
              "h-auto min-w-0 items-center justify-start gap-2 rounded-lg px-2.5 py-2 whitespace-normal sm:px-3 lg:items-start lg:gap-3 lg:py-3",
              props.value === category.value && "border-primary/20 text-foreground",
            )}
          >
            <span
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-lg lg:size-8",
                props.value === category.value
                  ? "bg-primary/10 text-primary"
                  : "bg-muted text-muted-foreground",
              )}
              aria-hidden="true"
            >
              <category.icon className="size-4" aria-hidden="true" />
            </span>
            <span className="min-w-0 text-left">
              <span className="block text-sm leading-5 font-medium break-words">
                {category.label}
              </span>
              <span
                id={`policy-category-description-${category.value}`}
                className="text-muted-foreground mt-1 hidden text-xs leading-5 font-normal break-words lg:block"
              >
                {category.description}
              </span>
            </span>
          </SegmentedControlItem>
        ))}
      </SegmentedControl>
    </div>
  );
}
