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

export function runItemMessage(item: {
  status: string;
  message: string;
  check?: Record<string, unknown>;
}): string {
  const error = checkError(item.check);
  if (item.status === "review" && error && !item.message.includes(error)) {
    return `${item.message}；检测出错：${error}`;
  }
  return item.message;
}
