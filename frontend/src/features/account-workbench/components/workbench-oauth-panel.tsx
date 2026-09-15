import { useState, type ReactElement } from "react";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { WorkbenchOAuth } from "./workbench-oauth";
import { WorkbenchOAuthBatch } from "./workbench-oauth-batch";
import { WorkbenchSMSReceipts } from "./workbench-sms-receipts";

export function WorkbenchOAuthPanel(): ReactElement {
  const [mode, setMode] = useState<"single" | "batch" | "profiles" | "receipts">("single");
  return (
    <div className="min-w-0 space-y-4">
      <SegmentedControl role="tablist" aria-label="授权方式">
        <SegmentedControlItem
          role="tab"
          id="oauth-single-mode"
          selected={mode === "single"}
          aria-controls="oauth-mode-content"
          onClick={() => setMode("single")}
        >
          单个授权
        </SegmentedControlItem>
        <SegmentedControlItem
          role="tab"
          id="oauth-batch-mode"
          selected={mode === "batch"}
          aria-controls="oauth-mode-content"
          onClick={() => setMode("batch")}
        >
          批量授权
        </SegmentedControlItem>
        <SegmentedControlItem
          role="tab"
          id="oauth-profiles-mode"
          selected={mode === "profiles"}
          aria-controls="oauth-mode-content"
          onClick={() => setMode("profiles")}
        >
          登录资料
        </SegmentedControlItem>
        <SegmentedControlItem
          role="tab"
          id="oauth-receipts-mode"
          selected={mode === "receipts"}
          aria-controls="oauth-mode-content"
          onClick={() => setMode("receipts")}
        >
          短信订单
        </SegmentedControlItem>
      </SegmentedControl>
      <section
        id="oauth-mode-content"
        role="tabpanel"
        aria-labelledby={`oauth-${mode}-mode`}
        className="min-w-0"
      >
        {mode === "single" && <WorkbenchOAuth />}
        {mode === "batch" && <WorkbenchOAuthBatch key="batch" />}
        {mode === "profiles" && <WorkbenchOAuthBatch key="profiles" reauthorization />}
        {mode === "receipts" && <WorkbenchSMSReceipts />}
      </section>
    </div>
  );
}
