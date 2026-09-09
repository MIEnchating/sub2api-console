import { BellRing, Link2, SlidersHorizontal, UsersRound } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { configTabs, type ConfigTab } from "../constants";

const tabIcons: Record<ConfigTab, LucideIcon> = {
  connection: Link2,
  accounts: UsersRound,
  notifications: BellRing,
  interface: SlidersHorizontal,
};

export function ConfigSectionTabs(props: {
  activeTab: ConfigTab;
  onTabChange?: (tab: ConfigTab) => void;
}) {
  return (
    <nav
      className="min-w-0 shrink-0"
      data-testid="system-settings-tabs"
      aria-label="系统设置分类导航"
    >
      <SegmentedControl
        className="grid w-full grid-cols-2 sm:grid-cols-4"
        role="tablist"
        aria-label="系统设置分类"
      >
        {configTabs.map((tab) => {
          const Icon = tabIcons[tab.value];
          return (
            <SegmentedControlItem
              key={tab.value}
              id={`config-tab-${tab.value}`}
              className="h-9 w-full justify-center gap-2"
              type="button"
              role="tab"
              selected={props.activeTab === tab.value}
              aria-controls={`config-panel-${tab.value}`}
              onClick={() => props.onTabChange?.(tab.value)}
            >
              <Icon className="size-3.5" aria-hidden="true" />
              {tab.label}
            </SegmentedControlItem>
          );
        })}
      </SegmentedControl>
    </nav>
  );
}
