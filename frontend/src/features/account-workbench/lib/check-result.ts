import { checkVerdictLabels } from "../constants";

export function checkError(check: Record<string, unknown> | undefined): string {
  if (typeof check?.error === "string" && check.error.trim()) return check.error.trim();
  if (check?.verdict === "ERROR") return "检测未完成，请查看检测详情后重试";
  return "";
}

export function checkLabel(check: Record<string, unknown> | undefined): string {
  if (checkError(check)) return checkVerdictLabels.ERROR;
  return checkVerdictLabels[String(check?.verdict)] || "待复核结论";
}

export function completedCheck(check: Record<string, unknown> | undefined): boolean {
  return Boolean(check && !checkError(check) && checkVerdictLabels[String(check.verdict)]);
}
