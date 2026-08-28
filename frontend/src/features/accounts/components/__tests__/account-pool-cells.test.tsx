import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { AccountStatus } from "@/api";
import {
  AccountHealthCell,
  AccountIdentityCell,
  AccountLatencyCell,
  AccountRecentResultsCell,
  AccountRoutingParametersCell,
  AccountStateCell,
} from "../account-pool-cells";

const account: AccountStatus = {
  id: "63005",
  name: "primary-account",
  groups: ["codex", "fallback"],
  upstream_host: "api.example.test",
  upstream_type: "apikey",
  platform: "openai",
  account_type: "apikey",
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
    expect(markup).toContain("最近错误：上游网关错误");
    expect(markup).not.toContain("已停止调度");
  });

  it("shows the latest error below a healthy status like Guardian", () => {
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
    expect(markup).toContain("最近错误：API returned 503: service unavailable");
  });

  it("shows the actual reason when Sub2API has stopped scheduling", () => {
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
    expect(markup).toContain("停止原因：连续凭据错误触发熔断");
    expect(markup).not.toContain(">已停止调度<");
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

  it("shows score detail, recent evidence and first-token latency", () => {
    const markup = renderToStaticMarkup(
      <>
        <AccountHealthCell account={account} />
        <AccountRecentResultsCell account={account} />
        <AccountLatencyCell account={account} />
      </>,
    );
    const latencyMarkup = renderToStaticMarkup(<AccountLatencyCell account={account} />);

    expect(markup).toContain("72.5");
    expect(markup).toContain('data-slot="account-health-score"');
    expect(markup).toContain("健康分 72.5");
    expect(markup).toContain("短期 68");
    expect(markup).toContain("长期 83");
    expect(markup).toContain('data-slot="account-recent-results"');
    expect(markup).toContain("2 条样本");
    expect(latencyMarkup).toContain("P95 1s");
    expect(latencyMarkup).toContain("P50 0s");
    expect(latencyMarkup).not.toContain("1.25s");
    expect(latencyMarkup).not.toContain("0.32s");
    expect(latencyMarkup).not.toContain("1250ms");
  });

  it("shows missing first-token latency without a seconds suffix", () => {
    const markup = renderToStaticMarkup(
      <AccountLatencyCell account={{ ...account, ttfb_p50_ms: null, ttfb_p95_ms: null }} />,
    );

    expect(markup).toContain("P95 —");
    expect(markup).toContain("P50 —");
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
    expect(markup).not.toContain("72.5");
  });

  it("labels current and target routing parameters instead of using an unexplained arrow", () => {
    const markup = renderToStaticMarkup(<AccountRoutingParametersCell account={account} />);

    expect(markup).toContain("当前优先级 12");
    expect(markup).toContain("目标优先级 9");
    expect(markup).toContain("负载");
    expect(markup).toContain("并发");
    expect(markup).not.toContain("→");
  });
});
