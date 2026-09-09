import { cleanup, render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";
import {
  AccountBaseURLCell,
  AccountHealthCell,
  AccountIdentityCell,
  AccountKeyStatusCell,
  AccountLatencyCell,
  AccountRecentResultsCell,
  AccountRoutingParametersCell,
  AccountStateCell,
} from "../account-pool-cells";

it("人工优先位状态使用中文展示且不显示历史成本墙拦截", () => {
  render(
    <AccountStateCell
      account={{
        ...account,
        manual_priority: 1,
        health: "manual_priority",
        routing_state: "manual_priority",
        health_status: "cost_blocked",
        decision_state: null,
        decision_reason: null,
      }}
    />,
  );

  expect(screen.getByText("人工优先位")).toBeVisible();
  expect(screen.getByText("调度开关：已开启")).toBeVisible();
  expect(screen.queryByText("成本墙拦截")).not.toBeInTheDocument();
  expect(screen.queryByText("待探测")).not.toBeInTheDocument();
});

const account: AccountStatus = {
  id: "63005",
  name: "primary-account",
  groups: ["codex", "fallback"],
  upstream_id: "up_test",
  upstream_host: "api.example.test",
  upstream_type: "apikey",
  platform: "openai",
  account_type: "apikey",
  base_url: "https://api.openai.com/v1",
  upstream_base_url: "https://gateway.example.test",
  base_url_check: "official_mismatch",
  base_url_check_reason:
    "上游地址不是官方服务，但账号 Base URL 指向官方地址，请检查添加账号时的 Base URL",
  key_status: "active",
  key_status_reason: "上游 Key key-1 状态为 active，所属分组仍存在",
  sub2api_status: "active",
  sub2api_error: null,
  schedulable: true,
  priority: 12,
  load_factor: "38",
  concurrency: 30,
  multiplier: "0.3",
  balance: "95",
  paused: false,
  paused_reason: null,
  routing_state: "observing",
  health_status: "失败",
  health: "degraded",
  desired_health: "degraded",
  apply_pending: false,
  apply_error: null,
  decision_state: "degraded",
  decision_reason: "健康分低于降级线 75",
  failure_streak: 2,
  recovery_pass_streak: 0,
  target_priority: 9,
  target_load_factor: "42",
  target_schedulable: true,
  target_concurrency: 30,
  health_score: 72.5,
  short_score: 68,
  long_score: 83,
  sample_count: 2,
  recent_results: [
    {
      result: "失败",
      observed_at: "2026-08-26T12:00:00Z",
      latency_ms: 1250,
      failure_reason: "上游网关错误",
      source: "traffic",
    },
    {
      result: "通过",
      observed_at: "2026-08-26T11:00:00Z",
      latency_ms: 320,
      failure_reason: null,
      source: "active-probe",
    },
  ],
  ttfb_p50_ms: 320,
  ttfb_p95_ms: 1250,
  weight: 88.4,
};

describe("account pool cells", () => {
  it("评分详情区分实际短长期样本数，不把配置上限当作样本数", () => {
    render(
      <AccountHealthCell
        account={{ ...account, sample_count: 58, short_sample_count: 10, long_sample_count: 58 }}
      />,
    );
    expect(screen.getByLabelText(/查看健康评分详情/)).toHaveAccessibleName(
      /短期样本数 10，长期样本数 58/,
    );
  });

  it("旧评估未记录短期样本数时显示未记录，零样本时显示零", () => {
    const view = render(<AccountHealthCell account={account} />);
    expect(screen.getByLabelText(/查看健康评分详情/)).toHaveAccessibleName(
      /短期样本数 未记录，长期样本数 2/,
    );
    view.rerender(<AccountHealthCell account={{ ...account, sample_count: 0 }} />);
    expect(screen.getByLabelText(/查看健康评分详情/)).toHaveAccessibleName(
      /短期样本数 0，长期样本数 0/,
    );
  });

  afterEach(cleanup);

  it("shows that a full-score account still has its scheduling switch closed", () => {
    render(
      <AccountStateCell
        account={{ ...account, health: "fused", health_score: 100, schedulable: false }}
      />,
    );
    expect(screen.getByText("调度开关：已关闭")).toBeVisible();
  });

  it("shows a warning and observation reason when a 40-point account awaits failure confirmation", () => {
    const pendingAccount = {
      ...account,
      health: "healthy",
      decision_state: "healthy",
      desired_health: "healthy",
      health_score: 40,
      evidence_pending: true,
      decision_reason: "短暂异常待确认，保持当前调度位置",
    };
    render(<AccountStateCell account={pendingAccount} />);

    expect(screen.getByText("异常待确认").closest("[data-slot='status-badge']")).toHaveClass(
      "text-warning",
    );
    expect(screen.getByText("观察原因：短暂异常待确认，保持当前调度位置")).toBeVisible();
    expect(screen.queryByText("健康", { exact: true })).not.toBeInTheDocument();
  });

  it("returns to a green healthy badge when the backend clears pending evidence", () => {
    const pendingAccount = {
      ...account,
      health: "healthy",
      decision_state: "healthy",
      evidence_pending: true,
      decision_reason: "短暂异常待确认，保持当前调度位置",
    };
    const view = render(<AccountStateCell account={pendingAccount} />);
    view.rerender(
      <AccountStateCell
        account={{
          ...pendingAccount,
          health_score: 100,
          evidence_pending: false,
          decision_reason: "已计算",
        }}
      />,
    );

    expect(
      screen.getByText("健康", { exact: true }).closest("[data-slot='status-badge']"),
    ).toHaveClass("text-success");
    expect(screen.queryByText("异常待确认")).not.toBeInTheDocument();
    expect(screen.queryByText(/^观察原因/)).not.toBeInTheDocument();
  });

  it("keeps the partial-weight explanation visible beside recovery progress for a degraded account", () => {
    render(
      <AccountStateCell
        account={{
          ...account,
          health_score: 40,
          evidence_pending: true,
          decision_reason: "健康分低于降级线 75；短暂异常待确认，暂时降低调度权重",
          recovery: {
            evaluated_at: "2026-09-07T12:00:00Z",
            ready: false,
            conditions: [{ code: "success_streak", met: false, detail: "连续成功 0/2 次" }],
          },
        }}
      />,
    );
    expect(screen.getByText("降级", { exact: true })).toBeVisible();
    expect(screen.queryByText("健康", { exact: true })).not.toBeInTheDocument();
    expect(screen.getByText(/短暂异常待确认，暂时降低调度权重/)).toBeVisible();
    expect(screen.getByText(/连续成功 0\/2 次/)).toBeVisible();
    expect(screen.getByText("调度开关：已开启")).toBeVisible();
  });

  it.each([
    { health: "fused", label: "已熔断" },
    { health: "paused", label: "已暂停" },
    { health: "degraded", label: "降级" },
  ])("keeps $label when stale pending evidence accompanies $health", (state) => {
    const pendingAccount = { ...account, health: state.health, evidence_pending: true };
    render(<AccountStateCell account={pendingAccount} />);

    expect(screen.getByText(state.label, { exact: true })).toBeVisible();
    expect(screen.queryByText("异常待确认")).not.toBeInTheDocument();
  });

  it("shows account identity as name, ID/type, Host, then groups", () => {
    const markup = renderToStaticMarkup(<AccountIdentityCell account={account} />);

    expect(markup.indexOf("primary-account")).toBeLessThan(markup.indexOf("#63005"));
    expect(markup.indexOf("#63005")).toBeLessThan(markup.indexOf("api.example.test"));
    expect(markup.indexOf("api.example.test")).toBeLessThan(
      markup.indexOf("分组：codex、fallback"),
    );
    expect(markup).toContain("API Key");
    expect(markup).toContain("OpenAI");
    expect(markup).toContain("truncate");
  });

  it("summarizes groups and shows normalized routing state", () => {
    const markup = renderToStaticMarkup(<AccountStateCell account={account} />);

    expect(markup).toContain("降级");
    expect(markup).toContain("降级原因：健康分低于降级线 75");
    expect(markup).not.toContain("最近错误：上游网关错误");
    expect(markup).toContain("text-warning");
    expect(markup).not.toContain("已停止调度");
  });

  it("does not show historical request errors while Sub2API is active", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          health: "healthy",
          decision_state: "healthy",
          decision_reason: null,
          last_error: "API returned 503: service unavailable",
        }}
      />,
    );

    expect(markup).toContain("健康");
    expect(markup).not.toContain("最近错误：API returned 503: service unavailable");
  });

  it("does not repeat a decision reason as a second scheduling stop reason", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          health: "fused",
          routing_state: "fused",
          schedulable: false,
          decision_state: "fused",
          decision_reason: "连续凭据错误触发熔断",
          upstream_block: "unschedulable",
          upstream_block_reason: "Sub2API 调度开关已关闭，但未记录触发原因",
        }}
      />,
    );

    expect(markup).toContain("熔断原因：连续凭据错误触发熔断");
    expect(markup).not.toContain("停止原因：连续凭据错误触发熔断");
    expect(markup).not.toContain(">已停止调度<");
  });

  it("shows Base URL validation and upstream Key status", () => {
    const markup = renderToStaticMarkup(
      <>
        <AccountBaseURLCell account={account} />
        <AccountKeyStatusCell account={account} />
      </>,
    );

    expect(markup).toContain("配置异常");
    expect(markup).toContain("https://api.openai.com/v1");
    expect(markup).toContain("上游访问地址：https://gateway.example.test");
    expect(markup).toContain("正常");
  });

  it("distinguishes an unchecked account from a checked detail without Base URL", () => {
    const unchecked = renderToStaticMarkup(
      <AccountBaseURLCell
        account={{
          ...account,
          base_url: null,
          base_url_check: "unchecked",
          base_url_checked_at: null,
        }}
      />,
    );
    const missing = renderToStaticMarkup(
      <AccountBaseURLCell
        account={{
          ...account,
          base_url: null,
          base_url_check: "unknown",
          base_url_checked_at: "2026-08-30T12:00:00Z",
        }}
      />,
    );

    expect(unchecked).toContain("尚未校验");
    expect(unchecked).toContain("等待校验");
    expect(missing).toContain("缺少账号 Base URL");
    expect(missing).toContain("详情未提供 Base URL");
  });

  it("does not call an existing account Base URL missing when upstream ownership is absent", () => {
    const markup = renderToStaticMarkup(
      <AccountBaseURLCell
        account={{
          ...account,
          base_url: "https://api.x.ai/v1",
          upstream_host: null,
          upstream_base_url: null,
          base_url_check: "unknown",
        }}
      />,
    );

    expect(markup).toContain("缺少上游信息");
    expect(markup).not.toContain("缺少账号 Base URL");
  });

  it("labels a resolved platform default Base URL", () => {
    const markup = renderToStaticMarkup(
      <AccountBaseURLCell
        account={{
          ...account,
          base_url: "https://api.openai.com",
          base_url_source: "platform_default",
        }}
      />,
    );

    expect(markup).toContain("来源：Sub2API 平台默认地址");
  });

  it("maps NewAPI and missing Key or group states to actionable labels", () => {
    const exhausted = renderToStaticMarkup(
      <AccountKeyStatusCell account={{ ...account, key_status: "4" }} />,
    );
    const missing = renderToStaticMarkup(
      <AccountKeyStatusCell account={{ ...account, key_status: "key_missing" }} />,
    );
    const suspected = renderToStaticMarkup(
      <AccountKeyStatusCell account={{ ...account, key_status: "suspected" }} />,
    );
    const groupMissing = renderToStaticMarkup(
      <AccountKeyStatusCell
        account={{
          ...account,
          key_status: "group_missing",
          key_status_reason: "上游 Key key-1 仍有绑定，但所属分组 pro 已删除或不存在",
        }}
      />,
    );

    expect(exhausted).toContain("额度耗尽");
    expect(suspected).toContain("待复核");
    expect(missing).toContain("Key 已删除");
    expect(groupMissing).toContain("分组已删除");
  });

  it("does not mistake an unexplained scheduling switch for the degraded reason", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          schedulable: false,
          upstream_block: "unschedulable",
          upstream_block_reason: "Sub2API 调度开关已关闭，但未记录触发原因",
        }}
      />,
    );

    expect(markup).toContain("降级原因：健康分低于降级线 75");
    expect(markup).toContain("停止原因未记录：Sub2API 调度开关已关闭");
    expect(markup).not.toContain("停止原因：健康分低于降级线 75");
    expect(markup).not.toContain("停止原因：Sub2API 调度开关已关闭");
  });

  it("shows a recorded fuse decision even while the displayed state is still catching up", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          schedulable: false,
          decision_state: "fused",
          decision_reason: "上游倍率 1.5 超过配置阈值 1.2",
          upstream_block: "unschedulable",
          upstream_block_reason: "Sub2API 调度开关已关闭，但未记录触发原因",
        }}
      />,
    );

    expect(markup).toContain("停止原因：上游倍率 1.5 超过配置阈值 1.2");
    expect(markup).not.toContain("停止原因未记录");
  });

  it("手动熔断返回人工处置原因时展示熔断原因而非原因未记录", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          health: "fused",
          schedulable: false,
          decision_state: "fused",
          decision_reason: "人工熔断，等待手动解除",
          upstream_block: "unschedulable",
          upstream_block_reason: "Sub2API 调度开关已关闭，但未记录触发原因",
        }}
      />,
    );

    expect(markup).toContain("熔断原因：人工熔断，等待手动解除");
    expect(markup).not.toContain("停止原因未记录");
  });

  it("shows temporary upstream scheduling blocks with their recovery reason", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          health: "healthy",
          schedulable: true,
          decision_state: "healthy",
          decision_reason: null,
          recent_results: [],
          upstream_block: "temp_unschedulable",
          upstream_block_reason: "令牌刷新失败，08-27 13:00 恢复",
        }}
      />,
    );

    expect(markup).toContain("停止原因：令牌刷新失败，08-27 13:00 恢复");
  });

  it("does not present a stale desired-state reason as the effective disabled reason", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          health: "disabled",
          schedulable: false,
          decision_state: "degraded",
          decision_reason: "健康分低于降级线 75",
          upstream_block: "disabled",
          upstream_block_reason: "账号已在 Sub2API 停用（状态 inactive），未记录停用原因",
        }}
      />,
    );

    expect(markup).toContain("停用原因：账号已在 Sub2API 停用（状态 inactive），未记录停用原因");
    expect(markup).not.toContain("停用原因：健康分低于降级线 75");
  });

  it("shows the desired state only in a tooltip while writeback is pending", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          health: "healthy",
          routing_state: "healthy",
          desired_health: "fused",
          decision_state: "fused",
          apply_pending: true,
          apply_error: "上游写回超时",
        }}
      />,
    );

    expect(markup).toContain("健康");
    expect(markup).toContain("引擎期望：已熔断");
    expect(markup).toContain("上游写回超时");
  });

  it("shows score detail, recent evidence and combined latency", () => {
    const markup = renderToStaticMarkup(
      <>
        <AccountHealthCell account={account} />
        <AccountRecentResultsCell account={account} />
        <AccountLatencyCell account={account} />
      </>,
    );
    const latencyMarkup = renderToStaticMarkup(<AccountLatencyCell account={account} />);

    expect(markup).toContain("73");
    expect(markup).toContain('data-slot="account-health-score"');
    expect(markup).toContain("健康分 73");
    expect(markup).toContain("短期评分 68");
    expect(markup).toContain("长期评分 83");
    expect(markup).toContain("查看健康评分详情");
    expect(markup).toContain("cursor-pointer");
    expect(markup).toContain("综合健康分 73");
    expect(markup).toContain("有效样本 2");
    expect(markup).toContain("连续失败 2");
    expect(markup).toContain("连续恢复 0");
    expect(markup).toContain('data-slot="account-recent-results"');
    expect(markup).toContain("有效样本 2");
    expect(latencyMarkup).toContain("1.25s");
    expect(latencyMarkup).toContain("320ms");
    expect(latencyMarkup).toContain('aria-label="真实流量首字延迟说明"');
    expect(latencyMarkup).not.toContain("0.32s");
    expect(latencyMarkup).not.toContain("1250ms");
  });

  it("shows zero effective samples beside health even when recent history exists", () => {
    const recentResults = Array.from({ length: 10 }, (_, index) => ({
      result: "通过",
      observed_at: `2026-08-26T10:${String(index).padStart(2, "0")}:00Z`,
      latency_ms: 100 + index,
      failure_reason: null,
      source: "traffic",
    }));
    const healthMarkup = renderToStaticMarkup(
      <AccountHealthCell account={{ ...account, sample_count: 0 }} />,
    );
    const recentMarkup = renderToStaticMarkup(
      <AccountRecentResultsCell
        account={{ ...account, sample_count: 0, recent_results: recentResults }}
      />,
    );

    expect(healthMarkup).toContain('aria-label="暂无健康分"');
    expect(healthMarkup).toContain(">有效样本 0");
    expect(recentMarkup.match(/tabindex="0"/g)).toHaveLength(10);
    expect(recentMarkup).not.toContain("有效样本 0");
    expect(recentMarkup).toContain("真实流量结果");
    expect(recentMarkup).not.toContain("条最近结果");
  });

  it("shows missing combined latency without a seconds suffix", () => {
    const markup = renderToStaticMarkup(
      <AccountLatencyCell account={{ ...account, ttfb_p50_ms: null, ttfb_p95_ms: null }} />,
    );

    expect(markup).toContain("P95</dt>");
    expect(markup).toContain("暂无首字数据");
    expect(markup).not.toContain("—s");
  });

  it("shows no score when the latest evaluation has no valid samples", () => {
    const markup = renderToStaticMarkup(
      <AccountHealthCell
        account={{
          ...account,
          health: "unknown",
          health_score: null,
          short_score: null,
          long_score: null,
          sample_count: 0,
        }}
      />,
    );

    expect(markup).toContain("暂无健康分");
    expect(markup).toContain("—");
    expect(markup).not.toContain("73");
  });

  it("labels current and target routing parameters instead of using an unexplained arrow", () => {
    const markup = renderToStaticMarkup(<AccountRoutingParametersCell account={account} />);

    expect(markup).toContain("当前优先级 12");
    expect(markup).toContain("目标优先级 9");
    expect(markup).toContain("负载");
    expect(markup).toContain("并发");
    expect(markup).not.toContain("→");
  });

  it("shows that manual priority only controls upstream balance syncing", () => {
    const withoutBalanceSync = renderToStaticMarkup(
      <AccountRoutingParametersCell
        account={{
          ...account,
          schedulable: false,
          manual_priority: 3,
          manual_sync_balance_multiplier: false,
        }}
      />,
    );
    const withBalanceSync = renderToStaticMarkup(
      <AccountRoutingParametersCell
        account={{ ...account, manual_priority: 3, manual_sync_balance_multiplier: true }}
      />,
    );

    expect(withoutBalanceSync).toContain("人工优先位 #3");
    expect(withoutBalanceSync).toContain("停止调度");
    expect(withoutBalanceSync).toContain("不同步上游余额");
    expect(withBalanceSync).toContain("参与调度");
    expect(withBalanceSync).toContain("同步上游余额");
  });
});
