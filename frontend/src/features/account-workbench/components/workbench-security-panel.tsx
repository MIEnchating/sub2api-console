import { useState, type ReactElement } from "react";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { WorkbenchSecurity } from "./workbench-security";
import { WorkbenchSecurityBatch } from "./workbench-security-batch";

export function WorkbenchSecurityPanel(): ReactElement {
  const [batch, setBatch] = useState(false);
  return (
    <div className="min-w-0 space-y-4">
      <SegmentedControl role="tablist" aria-label="安全设置方式">
        <SegmentedControlItem
          role="tab"
          id="security-single-mode"
          selected={!batch}
          aria-controls="security-mode-content"
          onClick={() => setBatch(false)}
        >
          单个账号
        </SegmentedControlItem>
        <SegmentedControlItem
          role="tab"
          id="security-batch-mode"
          selected={batch}
          aria-controls="security-mode-content"
          onClick={() => setBatch(true)}
        >
          批量账号
        </SegmentedControlItem>
      </SegmentedControl>
      <section
        id="security-mode-content"
        role="tabpanel"
        aria-labelledby={batch ? "security-batch-mode" : "security-single-mode"}
      >
        {batch ? <WorkbenchSecurityBatch /> : <WorkbenchSecurity />}
      </section>
    </div>
  );
}
