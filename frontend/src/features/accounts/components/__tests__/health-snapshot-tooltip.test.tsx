import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";
import { AccountHealthCell } from "../account-pool-cells";

const account: AccountStatus = {
  id: "41",
  name: "实时评分账号",
  groups: [],
  upstream_id: null,
  upstream_host: null,
  upstream_type: null,
  health: "healthy",
  schedulable: true,
  priority: 10,
  load_factor: "1",
  concurrency: 1,
  multiplier: "1",
  balance: null,
  paused: false,
  paused_reason: null,
  routing_state: "active",
  health_status: null,
  desired_health: null,
  apply_pending: false,
  apply_error: null,
  decision_state: null,
  decision_reason: null,
  target_priority: null,
  target_load_factor: null,
  target_schedulable: null,
  target_concurrency: null,
  health_score: 62.5,
  short_score: 25,
  long_score: 62.5,
  sample_count: 2,
  short_sample_count: 1,
  long_sample_count: 2,
  failure_streak: 0,
  recovery_pass_streak: 3,
  recent_results: [],
  ttfb_p50_ms: null,
  ttfb_p95_ms: null,
  weight: 80,
};

describe("实时健康评分说明", () => {
  it("存在实时评估时间时键盘提示显示当前证据评分与两项时间", async () => {
    const user = userEvent.setup();
    const current = {
      ...account,
      health_evaluated_at: "2026-09-17T08:00:03.123456789Z",
      health_evidence_at: "2026-09-17T08:00:02Z",
    };
    render(<AccountHealthCell account={current} />);

    await user.tab();

    expect(screen.getByLabelText(/查看健康评分详情/)).toHaveFocus();
    const tooltip = await screen.findByRole("tooltip", { name: "健康评分详情" });
    expect(tooltip).toHaveTextContent("当前证据评分");
    expect(tooltip).toHaveTextContent("调度状态与连续失败、连续恢复次数沿用最近一次调度评估");
    expect(tooltip).not.toHaveTextContent("本轮调度采用的健康评估");
    expect(within(tooltip).getByText("评估时间")).toBeVisible();
    expect(within(tooltip).getByText("最新证据时间")).toBeVisible();
    const times = within(tooltip).getAllByRole("time");
    expect(times[0]).toHaveAttribute("datetime", current.health_evaluated_at);
    expect(times[1]).toHaveAttribute("datetime", current.health_evidence_at);
    expect(times[0]).toHaveTextContent("2026");
    expect(times[1]).toHaveTextContent("2026");
  });

  it("实时评估没有最新证据时提示暂无有效证据", async () => {
    const user = userEvent.setup();
    const current = {
      ...account,
      health_score: null,
      short_score: null,
      long_score: null,
      sample_count: 0,
      health_evaluated_at: "2026-09-17T08:00:03Z",
      health_evidence_at: null,
    };
    render(<AccountHealthCell account={current} />);
    await user.tab();

    const tooltip = await screen.findByRole("tooltip", { name: "健康评分详情" });
    expect(tooltip).toHaveTextContent("当前证据评分");
    expect(tooltip).toHaveTextContent("暂无有效证据");
    expect(within(tooltip).getAllByRole("time")).toHaveLength(1);
  });

  it("旧接口缺少实时评估时间时保留原调度评估说明", async () => {
    const user = userEvent.setup();
    render(<AccountHealthCell account={account} />);
    await user.tab();

    const tooltip = await screen.findByRole("tooltip", { name: "健康评分详情" });
    expect(tooltip).toHaveTextContent("本轮调度采用的健康评估");
    expect(tooltip).not.toHaveTextContent("当前证据评分");
    expect(within(tooltip).queryByText("最新证据时间")).not.toBeInTheDocument();
  });
});
