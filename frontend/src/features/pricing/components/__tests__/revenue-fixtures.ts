import type { RevenueReport, RevenueRow, Task } from "@/api";

export const revenueRow: RevenueRow = {
  account_id: "41",
  account_name: "核算账号",
  local_group: "标准分组",
  upstream_host: "api.example.test",
  upstream_key_name: "核算密钥",
  account_cost: "10.000000",
  actual_cost: "10.000000",
  upstream_raw_cost: "5.000000",
  recharge_rate: "1.000000",
  upstream_cost: "5.000000",
  difference: "0.000000",
  revenue: "5.000000",
  category: "正常",
  note: "",
  attribution_level: "key",
};

export const revenueReport: RevenueReport = {
  report_date: "2026-09-08",
  timezone: "Asia/Shanghai",
  tolerance: "2.000000",
  rows: [],
  summaries: [],
  issues: [],
  comparable: 0,
  unavailable: 0,
  abnormal: 0,
  generated_at: "2026-09-09T00:00:00Z",
};

export function revenueTask(report: RevenueReport = revenueReport): Task {
  return {
    id: "revenue-layout",
    skill: "sub2api-billing-reconciliation",
    operation: "revenue-calculation",
    status: "succeeded",
    progress: 100,
    message: "核算完成",
    result: report,
    created_at: "2026-09-09T00:00:00Z",
    updated_at: "2026-09-09T00:00:01Z",
  };
}
