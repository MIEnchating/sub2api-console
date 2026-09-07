import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { AccountStatus } from "@/api";
import { AccountStateCell } from "../account-pool-cells";

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

describe.each([false, true])("Sub2API 当前错误（展开详情：%s）", (expanded) => {
  it("同步为错误状态时仅显示 Sub2API 原因，不使用历史请求或探针原因", () => {
    render(
      <AccountStateCell
        expanded={expanded}
        account={{
          ...account,
          sub2api_status: "error",
          sub2api_error: "  Access forbidden (403): quota exceeded  ",
          last_error: "旧请求错误",
        }}
      />,
    );
    expect(screen.getByText("最近错误：Access forbidden (403): quota exceeded")).toBeVisible();
    expect(
      screen.queryByText(/最近错误：旧请求错误|最近错误：上游网关错误/),
    ).not.toBeInTheDocument();
  });

  it.each(["active", "disabled", "expired", null, ""])(
    "当前状态为 %s 时即使留有历史错误也不显示最近错误",
    (status) => {
      render(
        <AccountStateCell
          expanded={expanded}
          account={{
            ...account,
            sub2api_status: status,
            sub2api_error: "残留的 Sub2API 原因",
            last_error: "旧请求错误",
          }}
        />,
      );
      expect(screen.queryByText(/^最近错误：/)).not.toBeInTheDocument();
      expect(screen.getByText("调度开关：已开启")).toBeVisible();
    },
  );

  it("错误状态未返回原因时说明原因缺失，不用历史失败填充", () => {
    render(
      <AccountStateCell
        expanded={expanded}
        account={{
          ...account,
          sub2api_status: " ERROR ",
          sub2api_error: "  ",
          last_error: "旧请求错误",
        }}
      />,
    );
    expect(
      screen.getByText("最近错误：Sub2API 未返回错误原因，请同步账号查看最新状态"),
    ).toBeVisible();
    expect(
      screen.queryByText(/最近错误：旧请求错误|最近错误：上游网关错误/),
    ).not.toBeInTheDocument();
  });

  it("同步恢复正常后立即移除错误原因，再次报错时显示新原因", () => {
    const view = render(
      <AccountStateCell
        expanded={expanded}
        account={{ ...account, sub2api_status: "error", sub2api_error: "当前额度不足" }}
      />,
    );
    expect(screen.getByText("最近错误：当前额度不足")).toBeVisible();
    view.rerender(
      <AccountStateCell
        expanded={expanded}
        account={{
          ...account,
          sub2api_status: "active",
          sub2api_error: "当前额度不足",
          last_error: "历史额度不足",
        }}
      />,
    );
    expect(screen.queryByText(/^最近错误：/)).not.toBeInTheDocument();
    view.rerender(
      <AccountStateCell
        expanded={expanded}
        account={{ ...account, sub2api_status: "error", sub2api_error: "凭据已失效" }}
      />,
    );
    expect(screen.getByText("最近错误：凭据已失效")).toBeVisible();
    expect(screen.queryByText("最近错误：当前额度不足")).not.toBeInTheDocument();
  });
});
