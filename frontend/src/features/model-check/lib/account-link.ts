import type { AccountStatus } from "@/api";

export function accountCheckStatus(account: AccountStatus): {
  label: string;
  variant: "secondary" | "warning" | "destructive" | "outline";
} {
  switch (account.model_check_status) {
    case "loading":
      return { label: "读取中", variant: "outline" };
    case "unavailable":
      return { label: "结果读取失败", variant: "warning" };
    case "consistent":
      return { label: "符合特征", variant: "secondary" };
    case "inconsistent":
      return { label: "不符合特征", variant: "destructive" };
    case "inconclusive":
      return { label: "无法判定", variant: "warning" };
    default:
      return { label: "未检测", variant: "outline" };
  }
}

export function modelCheckSearch(search: Record<string, unknown>): { account_id?: string } {
  let id = search.account_id;
  if (typeof id === "number" && Number.isSafeInteger(id) && id > 0) id = String(id);
  return typeof id === "string" && /^[1-9]\d*$/.test(id) ? { account_id: id } : {};
}
