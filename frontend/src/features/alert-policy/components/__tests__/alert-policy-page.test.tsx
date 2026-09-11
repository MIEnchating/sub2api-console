import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { cacheAlertPolicyPageData, policy } from "./fixtures";
import { AlertPolicyPage } from "../alert-policy-page";

describe("AlertPolicyPage", () => {
  it("shows every persisted detection and delivery control", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    cacheAlertPolicyPageData(queryClient);

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
      "倍率上涨通知",
      "倍率下降通知",
      "余额不足",
      "主动探测失败",
      "账号熔断判定",
      "账号降级",
      "健康分过低",
      "网关错误率过高",
      "响应延迟超标",
      "其他降级原因",
      "保底强留",
      "分组无可调度账号",
      "分组仅剩保底账号",
      "自动执行失败",
      "配置异常恢复",
      "鉴权恢复",
      "倍率同步恢复",
      "余额恢复",
      "主动探测恢复",
      "账号熔断恢复",
      "账号降级恢复",
      "保底强留恢复",
      "分组可用性恢复",
      "分组保底恢复",
      "自动执行恢复",
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
    expect(markup).toContain('role="combobox"');
    expect(markup).toContain('aria-label="主动探测告警分组"');
    expect(markup).toContain(">codex<");
    expect(markup).toContain(">pro<");
    expect(markup).not.toContain("多个分组用逗号分隔");
    expect(markup).toContain("重复提醒间隔");
    expect(markup).toContain("状态变化冷却");
    expect(markup).toContain("多少条以上合并发送");
    for (const label of [
      "余额告警阈值",
      "连续主动探测失败次数",
      "连续主动探测成功次数",
      "主动探测告警分组",
      "重复提醒间隔（分钟）",
      "状态变化冷却（分钟）",
      "多少条以上合并发送",
    ]) {
      expect(markup).toContain(`aria-label="${label}说明"`);
    }
    for (const label of ["启用告警检测", "启用通知发送", "发送恢复通知", "恢复通知类型"]) {
      expect(markup).toContain(`aria-label="${label}说明"`);
    }
    expect(markup).not.toContain("达到次数后才产生主动探测告警。");
    expect(markup).not.toContain(
      "默认关闭容易频繁波动或无需闭环确认的恢复消息，告警记录仍会正常更新。",
    );
    expect(markup).toContain("告警检测");
    expect(markup).toContain("通知发送");
    expect(markup).not.toContain("运行控制");
    expect(markup).toContain('aria-label="管理通知渠道"');
    expect(markup).toContain('data-slot="notification-channel-summary"');
    expect(markup).toContain("QQBot · 私聊");
    expect(markup).not.toContain("当前支持 QQBot 通知渠道");
    expect(markup).not.toContain("敏感凭据继续由系统设置统一管理");
    expect(markup).not.toContain("目标类型：c2c");
    expect(markup).not.toContain("App ID");
    expect(markup).not.toContain("Client Secret");
    expect(markup).not.toContain("目标 ID");
    for (const label of ["上游与余额", "账号健康", "分组状态", "自动执行"]) {
      expect(markup).toContain(`>${label}</h3>`);
    }
    expect(markup.indexOf(">主动探测失败<")).toBeGreaterThan(markup.indexOf(">账号健康</h3>"));
    expect(markup.indexOf(">主动探测失败<")).toBeLessThan(markup.indexOf(">分组状态</h3>"));
    expect(markup.indexOf(">分组无可调度账号<")).toBeGreaterThan(markup.indexOf(">分组状态</h3>"));
    expect(markup.indexOf(">自动执行失败<")).toBeGreaterThan(markup.indexOf(">自动执行</h3>"));
    expect(markup).toContain('data-slot="routing-degraded-rules"');
    expect(markup).toContain('data-slot="recovery-notification-types"');
    expect(markup).not.toContain("min-h-44");
  });

  it("shows an empty active probe alert group selection as all configured groups", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    cacheAlertPolicyPageData(queryClient, { ...policy, probe_groups: [] });

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AlertPolicyPage onOpenSettings={() => undefined} />
      </QueryClientProvider>,
    );

    expect(markup).toContain(">全部分组<");
    expect(markup).toContain('role="combobox"');
    expect(markup).not.toContain('name="probe_groups"');
  });

  it("blocks policy editing and offers retry when the policy query fails", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    await queryClient.prefetchQuery({
      queryKey: ["alert-policy"],
      queryFn: async () => {
        throw new Error("读取失败");
      },
      retry: false,
    });

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <AlertPolicyPage onOpenSettings={() => undefined} />
      </QueryClientProvider>,
    );

    expect(markup).not.toContain('role="alert"');
    expect(markup).not.toContain("读取失败");
    expect(markup).not.toContain("告警策略暂不可用");
    expect(markup).toContain('aria-label="刷新告警策略"');
    expect(markup).not.toContain('data-slot="alert-policy-columns"');
    expect(markup).toMatch(
      /<button(?=[^>]*data-testid="alert-policy-reset")(?=[^>]*disabled="")[^>]*>/,
    );
    expect(markup).toMatch(
      /<button(?=[^>]*data-testid="alert-policy-save")(?=[^>]*disabled="")[^>]*>/,
    );
  });
});
