import type { WorkbenchPreview, WorkbenchPreviewItem } from "@/api";
import { previewActionLabels } from "../constants";

type PreviewSummary = {
  title: string;
  retry: boolean;
  scope: string;
  description: string;
  detection: string;
  notice: string;
};

function accountIDs(items: WorkbenchPreviewItem[]): string {
  return (
    items
      .map((item) => item.account_id)
      .filter(Boolean)
      .join("、") || "无"
  );
}

export function previewActionLabel(item: WorkbenchPreviewItem): string {
  if (item.action === "check" || item.action === "reconcile")
    return `${previewActionLabels[item.action]}（ID ${item.account_id}）`;
  return item.duplicate ? `更新凭据（ID ${item.account_id}）` : "新增账号";
}

export function summarizePreview(preview: WorkbenchPreview): PreviewSummary {
  if (preview.export_only && preview.scope === "local-export")
    return {
      title: "本地私有转换预览",
      retry: false,
      scope: `转换 ${preview.items.length} 项账号；独立导出范围。`,
      description: "保留输入配置并生成服务器私有 JSON 文件。",
      detection: "转换过程不执行模型检测。",
      notice: "文件保留 24 小时，包含完整账号凭据，仅保存在服务器私有目录。",
    };
  if (preview.export_only)
    return {
      title: "私有转换预览",
      retry: false,
      scope: `转换 ${preview.items.length} 项账号；模板所属目标：${preview.target}。`,
      description: "按预览配置生成服务器私有 JSON 文件。",
      detection: "转换过程不执行模型检测。",
      notice: "文件保留 24 小时，包含完整账号凭据，仅保存在服务器私有目录。",
    };
  const retry = preview.items.some((item) => item.action !== undefined);
  const imports = preview.items.filter((item) => !item.action || item.action === "import");
  const updates = imports.filter((item) => item.duplicate);
  const checks = preview.items.filter((item) => item.action === "check");
  const reconciles = preview.items.filter((item) => item.action === "reconcile");
  const configured = preview.items.filter((item) => item.action !== "reconcile");
  const templateNames =
    [...new Set(configured.map((item) => item.template_name || "默认配置"))].join("、") ||
    "不应用模板";
  const groups = [...new Set(configured.flatMap((item) => item.group_ids))].join("、") || "未分组";
  let detection = "导入后不执行模型检测，账号保持不可调度。";
  if (preview.check_after_import && configured.length > 0)
    detection = `使用 ${preview.model} 检测，会产生模型调用用量。`;
  if (reconciles.length > 0) {
    detection =
      configured.length > 0
        ? `${detection} 只读核对不执行模型调用。`
        : "只读核对不执行模型调用，不改变线上账号配置。";
  }
  let scope = `目标：${preview.target}；新增 ${imports.length - updates.length} 个账号；更新凭据 ${updates.length} 个。`;
  let description = `将 ${preview.items.length} 个账号导入 ${preview.target}，其中更新已有账号 ${updates.length} 个（ID：${accountIDs(updates)}）。模板：${templateNames}；分组 ID：${groups}。${detection} 未检测或检测未通过的账号保持不可调度。`;
  let notice =
    "账号先停止调度；未检测或检测未通过时保持不可调度。已有账号仅更新凭据，需按下方稳定 ID 确认目标。";
  if (retry) {
    scope = `目标：${preview.target}；新增 ${imports.length - updates.length} 个；更新凭据 ${updates.length} 个；重新检测 ${checks.length} 个；只读核对 ${reconciles.length} 个。`;
    description = `重新处理 ${preview.items.length} 个账号，目标 ${preview.target}。新增 ${imports.length - updates.length} 个；更新凭据 ${updates.length} 个（ID：${accountIDs(updates)}）；重新检测 ${checks.length} 个（ID：${accountIDs(checks)}）；只读核对 ${reconciles.length} 个（ID：${accountIDs(reconciles)}）。模板：${templateNames}；分组 ID：${groups}。${detection}`;
    notice =
      "重新检测复用线上凭据，检测通过并复核配置后启用；只读核对保留线上配置，仅确认原任务结果。";
  }
  return {
    title: retry ? "重新处理预览" : "导入预览",
    retry,
    scope,
    description,
    detection,
    notice,
  };
}
