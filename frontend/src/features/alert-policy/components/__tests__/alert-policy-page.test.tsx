import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { AlertPolicy, NotificationStatus } from "@/api";
import { AlertPolicyPage } from "../alert-policy-page";

const policy: AlertPolicy = {
  enabled: true,
  configuration_enabled: true,
  auth_enabled: true,
  rate_sync_enabled: true,
  balance_enabled: true,
  probe_enabled: true,
  routing_breaker_enabled: true,
  routing_degraded_enabled: true,
  routing_survivor_enabled: true,
  group_unavailable_enabled: true,
  group_survivor_enabled: true,
  apply_failure_enabled: true,
  balance_thresholds: ["20", "10", "5"],
  probe_failure_streak: 3,
  probe_recovery_streak: 3,
  probe_groups: ["codex", "pro"],
  delivery_enabled: true,
  notify_recovery: false,
  repeat_interval_minutes: 30,
  state_change_cooldown_minutes: 30,
  merge_threshold: 10,
};

describe("AlertPolicyPage", () => {
  it("shows every persisted detection and delivery control", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    queryClient.setQueryData(["alert-policy"], policy);
    queryClient.setQueryData<NotificationStatus>(["notification-status"], {
      configured: true,
      app_id: "app",
      client_secret_configured: true,
      home_channel: "target",
      channel_type: "c2c",
      destination_configured: true,
      configuration_errors: [],
      queues: {
        producer_firing: 0,
        producer_recovered: 0,
        consumer_pending: 0,
        consumer_failed: 0,
        consumer_active: false,
      },
    });

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AlertPolicyPage onOpenSettings={() => undefined} />
      </QueryClientProvider>,
    );

    for (const label of [
      "启用告警检测",
      "启用通知发送",
      "发送恢复通知",
      "配置异常",
      "鉴权失效",
      "倍率同步失败",
      "余额不足",
      "主动探测失败",
      "账号熔断判定",
      "账号降级",
      "保底强留",
      "分组无可调度账号",
      "分组仅剩保底账号",
      "自动执行失败",
    ]) {
      expect(markup).toContain(`aria-label="${label}"`);
    }
    expect(markup).toContain("余额告警阈值");
    expect(markup).toContain("添加阈值");
    expect(markup).toContain('aria-label="余额告警阈值 3"');
    expect(markup).toContain('data-slot="balance-threshold-list"');
    expect(markup).toContain("flex flex-wrap items-start gap-2");
    expect(markup.match(/pr-9/g)).toHaveLength(3);
    expect(markup.match(/absolute inset-y-0 right-1 z-10 my-auto size-7/g)).toHaveLength(3);
    expect(markup).not.toContain("-translate-y-1/2");
    expect(markup).not.toContain("sm:grid-cols-3");
    expect(markup).toContain("连续主动探测失败次数");
    expect(markup).toContain("连续主动探测成功次数");
    expect(markup).toContain("主动探测告警分组");
    expect(markup).toContain("重复提醒间隔");
    expect(markup).toContain("状态变化冷却");
    expect(markup).toContain("多少条以上合并发送");
    expect(markup).toContain("告警检测");
    expect(markup).toContain("通知发送");
    expect(markup.indexOf("告警检测")).toBeLessThan(markup.indexOf("阈值与范围"));
    expect(markup.indexOf("通知渠道")).toBeLessThan(markup.indexOf("通知发送"));
    expect(markup.indexOf("通知发送")).toBeLessThan(markup.indexOf("检测规则"));
    expect(markup).not.toContain("运行控制");
    expect(markup).toContain("管理通知渠道");
    expect(markup).toContain("目标类型：私聊");
    expect(markup).not.toContain("目标类型：c2c");
    expect(markup).not.toContain("App ID");
    expect(markup).not.toContain("Client Secret");
    expect(markup).not.toContain("目标 ID");
    expect(markup).toContain('data-slot="alert-policy-columns"');
    expect(markup).toContain("grid items-start gap-4 lg:grid-cols-2");
    expect(markup).toContain('data-slot="alert-delivery-switches"');
    expect(markup).toContain('data-slot="alert-delivery-fields"');
    expect(markup).toContain("xl:grid-cols-3");
    expect(markup).toContain('data-slot="alert-rule-grid"');
    expect(markup).toContain("xl:grid-cols-2");
    expect(markup).not.toContain("min-h-44");
  });
});
