import type { ReactElement, ReactNode } from "react";
import { SegmentedControl } from "@/components/ui/segmented-control";

export function WorkbenchModeTabs(props: { label: string; children: ReactNode }): ReactElement {
  return (
    <SegmentedControl
      role="tablist"
      aria-label={props.label}
      className="w-full flex-wrap rounded-none border-0 border-b bg-transparent p-0 pb-2"
    >
      {props.children}
    </SegmentedControl>
  );
}
