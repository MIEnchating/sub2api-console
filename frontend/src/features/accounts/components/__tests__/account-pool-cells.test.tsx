import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

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
  AccountSub2APIStatusCell,
} from "../account-pool-cells";

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
    expect(markup).toContain("text-warning");
    expect(markup).toContain("text-destructive");
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
    expect(markup).toContain("text-destructive");
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

  it("mirrors Sub2API account status and exposes its error detail", () => {
    const active = renderToStaticMarkup(<AccountSub2APIStatusCell account={account} />);
    const paused = renderToStaticMarkup(
      <AccountSub2APIStatusCell account={{ ...account, schedulable: false }} />,
    );
    const failed = renderToStaticMarkup(
      <AccountSub2APIStatusCell
        account={{
          ...account,
          sub2api_status: "error",
          sub2api_error: "Access forbidden (403): quota exceeded",
        }}
      />,
    );

    expect(active).toContain("正常");
    expect(paused).toContain("暂停");
    expect(failed).toContain("错误");
    expect(failed).toContain("查看 Sub2API 账号报错");
  });

  it("does not repeat the Sub2API error in the calculated state column", () => {
    const markup = renderToStaticMarkup(
      <AccountStateCell
        account={{
          ...account,
          sub2api_status: "error",
          sub2api_error: "Access forbidden (403): quota exceeded",
          last_error: "Access forbidden (403): quota exceeded",
          recent_results: [],
        }}
      />,
    );

    expect(markup).not.toContain("Access forbidden (403): quota exceeded");
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

  it("shows score detail, recent evidence and combined latency", () => {
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
    expect(markup).toContain("评分构成");
    expect(markup).toContain('aria-label="查看健康分评分构成"');
    expect(markup).toContain('data-slot="account-recent-results"');
    expect(markup).toContain("2 条样本");
    expect(latencyMarkup).toContain("P95 1s");
    expect(latencyMarkup).toContain("P50 0s");
    expect(latencyMarkup).toContain('aria-label="综合延迟"');
    expect(latencyMarkup).not.toContain("1.25s");
    expect(latencyMarkup).not.toContain("0.32s");
    expect(latencyMarkup).not.toContain("1250ms");
  });

  it("shows missing combined latency without a seconds suffix", () => {
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

  it("shows that manual priority only controls upstream balance syncing", () => {
    const withoutBalanceSync = renderToStaticMarkup(
      <AccountRoutingParametersCell
        account={{ ...account, manual_priority: 3, manual_sync_balance_multiplier: false }}
      />,
    );
    const withBalanceSync = renderToStaticMarkup(
      <AccountRoutingParametersCell
        account={{ ...account, manual_priority: 3, manual_sync_balance_multiplier: true }}
      />,
    );

    expect(withoutBalanceSync).toContain("人工优先位 #3");
    expect(withoutBalanceSync).toContain("不同步上游余额");
    expect(withBalanceSync).toContain("同步上游余额");
  });
});
