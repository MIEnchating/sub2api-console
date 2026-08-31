import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { AccountDetail } from "@/api";
import { accountDetailDialogLayout } from "../account-detail-dialog";
import { AccountSettingsPanel } from "../account-settings-panel";

const detail: AccountDetail = {
  id: "41",
  name: "channel-41",
  groups: ["codex"],
  upstream_id: "up_test",
  upstream_host: "api.example.test",
  upstream_type: "apikey",
  schedulable: true,
  priority: 753,
  load_factor: "1",
  concurrency: 3000,
  multiplier: "0.08",
  balance: null,
  paused: false,
  paused_reason: null,
  routing_state: "healthy",
  health_status: "healthy",
  health: "healthy",
  desired_health: "healthy",
  apply_pending: false,
  apply_error: null,
  decision_state: "healthy",
  decision_reason: null,
  failure_streak: 0,
  recovery_pass_streak: 0,
  target_priority: 120,
  target_load_factor: "1",
  target_schedulable: true,
  target_concurrency: 3000,
  health_score: 100,
  short_score: 100,
  long_score: 100,
  sample_count: 1,
  recent_results: [],
  ttfb_p50_ms: 100,
  ttfb_p95_ms: 120,
  weight: 100,
  metadata: {},
  group_rates: { codex: "0.08" },
  group_ids: { codex: "7" },
  bindings: [],
  test_model: "gpt-5.1-codex",
};

function renderPanel() {
  const queryClient = new QueryClient();
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <AccountSettingsPanel
        accountId="41"
        query={{ data: detail, isLoading: false, isError: false, error: null }}
        onCancel={() => undefined}
        onSaved={() => undefined}
      />
    </QueryClientProvider>,
  );
}

describe("账号设置面板", () => {
  it("matches channel settings and omits group editing and multiplier breaker fields", () => {
    const markup = renderPanel();

    for (const label of [
      "优先级",
      "负载因子",
      "并发上限",
      "倍率",
      "暂停调度",
      "排除该账号",
      "探测模型",
    ]) {
      expect(markup).toContain(label);
    }
    expect(markup).not.toContain("账号分组");
    expect(markup).not.toContain("倍率超阈值");
    expect(markup).not.toContain("账号名称");
    expect(markup).not.toContain("备注");
  });

  it("makes the complete pause and exclude rows operable buttons", () => {
    const markup = renderPanel();

    expect(markup).toContain('aria-label="暂停调度"');
    expect(markup).toContain('aria-label="排除该账号"');
    expect(markup).toContain('for="account-settings-41-paused"');
    expect(markup).toContain('for="account-settings-41-excluded"');
  });

  it("uses a compact two-column form without nested highlight cards", () => {
    const markup = renderPanel();

    expect(accountDetailDialogLayout.content).not.toContain("sm:max-w-");
    expect(markup).toContain('data-testid="account-routing-grid"');
    expect(markup).toContain("sm:grid-cols-2");
    expect(markup).toContain('data-testid="account-control-group"');
    expect(markup).not.toContain("border-primary/25");
    expect(markup).not.toContain("bg-primary/5");
  });
});
