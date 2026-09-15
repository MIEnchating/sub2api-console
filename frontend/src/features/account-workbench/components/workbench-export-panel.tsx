import { useState, type ReactElement } from "react";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { WorkbenchExports } from "./workbench-exports";
import { WorkbenchImport } from "./workbench-import";

export function WorkbenchExportPanel(): ReactElement {
  const [convert, setConvert] = useState(false);
  return (
    <div className="min-w-0 space-y-4">
      <SegmentedControl role="tablist" aria-label="私有文件来源">
        <SegmentedControlItem
          role="tab"
          id="export-existing"
          selected={!convert}
          aria-controls="export-source-content"
          onClick={() => setConvert(false)}
        >
          线上账号导出
        </SegmentedControlItem>
        <SegmentedControlItem
          role="tab"
          id="export-conversion"
          selected={convert}
          aria-controls="export-source-content"
          onClick={() => setConvert(true)}
        >
          输入转换为 JSON
        </SegmentedControlItem>
      </SegmentedControl>
      <section
        id="export-source-content"
        role="tabpanel"
        className="min-w-0"
        aria-labelledby={convert ? "export-conversion" : "export-existing"}
      >
        {convert ? <WorkbenchImport output="export" /> : <WorkbenchExports />}
      </section>
    </div>
  );
}
