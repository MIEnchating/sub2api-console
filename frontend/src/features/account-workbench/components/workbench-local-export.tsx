import { useState, type ReactElement } from "react";
import { SegmentedControlItem } from "@/components/ui/segmented-control";
import { WorkbenchModeTabs } from "./workbench-mode-tabs";
import { WorkbenchImport } from "./workbench-import";
import { WorkbenchOAuth } from "./workbench-oauth";
import { WorkbenchOAuthBatch } from "./workbench-oauth-batch";
import { WorkbenchMixed } from "./workbench-mixed";
import { WorkbenchExportArtifacts } from "./workbench-export-artifacts";
import { WorkbenchSMSReceipts } from "./workbench-sms-receipts";
import { WorkbenchScopeNotice } from "./workbench-scope-notice";

const modes = [
  { id: "json", label: "JSON / RT 转换" },
  { id: "oauth", label: "单个授权" },
  { id: "batch", label: "批量授权" },
  { id: "mixed", label: "混合运行" },
  { id: "files", label: "私有文件" },
  { id: "receipts", label: "短信订单" },
  { id: "profiles", label: "登录资料" },
] as const;

export function WorkbenchLocalExport(): ReactElement {
  const [mode, setMode] = useState<(typeof modes)[number]["id"]>("json");
  return (
    <div className="min-w-0 space-y-4">
      <WorkbenchScopeNotice scope="local-export">
        只生成服务器私有文件，不读取或修改线上托管账号。
      </WorkbenchScopeNotice>
      <WorkbenchModeTabs label="本地导出来源">
        {modes.map((item) => (
          <SegmentedControlItem
            key={item.id}
            id={`local-export-${item.id}`}
            role="tab"
            selected={mode === item.id}
            aria-controls="local-export-content"
            onClick={() => setMode(item.id)}
          >
            {item.label}
          </SegmentedControlItem>
        ))}
      </WorkbenchModeTabs>
      <section
        id="local-export-content"
        role="tabpanel"
        aria-labelledby={`local-export-${mode}`}
        className="min-w-0"
      >
        {mode === "json" && <WorkbenchImport output="export" scope="local-export" />}
        {mode === "oauth" && <WorkbenchOAuth scope="local-export" />}
        {mode === "batch" && <WorkbenchOAuthBatch scope="local-export" />}
        {mode === "mixed" && <WorkbenchMixed scope="local-export" />}
        {mode === "files" && <WorkbenchExportArtifacts scope="local-export" />}
        {mode === "receipts" && <WorkbenchSMSReceipts scope="local-export" />}
        {mode === "profiles" && <WorkbenchOAuthBatch scope="local-export" localProfiles />}
      </section>
    </div>
  );
}
