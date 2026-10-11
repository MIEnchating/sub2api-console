import type { WorkbenchAccount, WorkbenchRun } from "../types";

export function accountListFixture(count: number): WorkbenchAccount[] {
  return Array.from({ length: count }, (_, index) => ({
    id: String(index + 1),
    name: `账号 ${index + 1}`,
    email: `account-${index + 1}@example.test`,
    status: "active",
    schedulable: true,
    plan: "",
    groups: [],
    proxy_name: "",
    concurrency: "1",
    load_factor: "",
    rate_multiplier: "1",
    model_mapping: {},
    fingerprint: "",
  }));
}

export function runItemsFixture(count: number): WorkbenchRun {
  return {
    id: "batch-one",
    revision: 1,
    task_id: "task-one",
    action: "import",
    status: "needs_attention",
    created_at: "2026-10-10T00:00:00Z",
    updated_at: "2026-10-10T00:00:00Z",
    expires_at: "2099-01-01T00:00:00Z",
    duplicate_count: 0,
    items: accountListFixture(count).map((account, index) => ({
      id: account.id,
      index,
      kind: "codex_json",
      name: account.name,
      email: account.email,
      plan: "",
      identity_source: "official_signature",
      status: "review",
      message: "检测未通过",
      account_id: account.id,
      template_name: "默认配置",
      check: { verdict: "INCONCLUSIVE" },
    })),
  };
}
