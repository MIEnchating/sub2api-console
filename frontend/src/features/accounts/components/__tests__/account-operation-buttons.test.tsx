import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import type { AccountStatus } from "@/api";
import { AccountOperationButtons } from "../account-operation-buttons";

const account: AccountStatus = {
  id: "41",
  name: "channel-41",
  groups: ["codex"],
  upstream_host: "api.example.test",
  upstream_type: "apikey",
  schedulable: true,
  priority: 1,
  load_factor: "10",
  concurrency: 2,
  multiplier: "0.1",
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
  target_priority: 1,
  target_load_factor: "10",
  target_schedulable: true,
  target_concurrency: 2,
  health_score: 100,
  short_score: 100,
  long_score: 100,
  sample_count: 1,
  recent_results: [],
  ttfb_p50_ms: 100,
  ttfb_p95_ms: 120,
  weight: 100,
};

function markup(overrides: Partial<AccountStatus> = {}, probePending = false) {
  return renderToStaticMarkup(
    <AccountOperationButtons
      account={{ ...account, ...overrides }}
      pending={probePending}
      probePending={probePending}
      onProbe={vi.fn()}
      onControl={vi.fn()}
      onRateSync={vi.fn()}
      onEdit={vi.fn()}
    />,
  );
}

describe("account operation buttons", () => {
  it("matches the channel pool operations without a multiplier threshold breaker", () => {
    const result = markup();
    for (const label of ["探活测试", "暂停调度", "手动熔断", "同步上游倍率", "查看并编辑账号"]) {
      expect(result).toContain(label);
    }
    expect(result).toContain("grid-cols-2");
    expect(result).toContain("col-span-2");
    expect(result).toContain("lucide-activity");
    expect(result).not.toContain("lucide-scan-search");
    expect(result).not.toContain("排除账号");
    expect(result).not.toContain("倍率超阈值");
  });

  it("shows an immediate loading state while an active probe is running", () => {
    const result = markup({}, true);

    expect(result).toContain(`aria-label="正在探活 ${account.name}"`);
    expect(result).toContain("lucide-loader-circle");
    expect(result).toContain("animate-spin");
    expect(result).toContain("disabled");
    expect(result).not.toContain("lucide-activity");
  });

  it("switches pause and fuse actions to their recovery variants", () => {
    expect(markup({ health: "paused", routing_state: "paused", paused: true })).toContain(
      "恢复调度",
    );
    expect(markup({ health: "fused", routing_state: "fused", schedulable: false })).toContain(
      "解除熔断",
    );
  });

  it("only offers restore control and edit for an excluded account", () => {
    const result = markup({ health: "excluded", routing_state: "excluded", schedulable: false });
    expect(result).toContain("恢复管控");
    expect(result).toContain("查看并编辑账号");
    expect(result).not.toContain("探活测试");
    expect(result).not.toContain("手动熔断");
  });
});
