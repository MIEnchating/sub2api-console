import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  ConfigPage,
  notificationFormFromStatus,
  notificationTargetResultFromTask,
  notificationTargetField,
  targetFormFromConfig,
} from "../App";
import type { NotificationStatus, RuntimeConfig, Task } from "../api";

describe("系统设置页面职责", () => {
  it("只展示可操作的系统设置，不重复展示运行状态和 Host 鉴权配置", () => {
    const queryClient = new QueryClient();
    const config: RuntimeConfig = {
      database_path: "/data/sub2api-console.sqlite3",
      data_database_path: "/data/sub2api-console.sqlite3",
      database_available: true,
      data_database_available: true,
      mode: "完全模式",
      config_keys: [],
      secret_values_hidden: true,
      probes_enabled: true,
      account_default_concurrency: 10,
      account_default_priority: 1,
      admin_base_url: "https://sub2api.example.test",
      request_timeout_seconds: 60,
      initialized: true,
      target_configured: true,
      console_username: "admin",
      configuration_errors: [],
    };
    const notifications: NotificationStatus = {
      configured: true,
      app_id: "configured-app",
      client_secret_configured: true,
      home_channel: "configured-target",
      channel_type: "c2c",
      destination_configured: true,
      configuration_errors: [],
      queues: {
        producer_firing: 2,
        producer_recovered: 1,
        consumer_pending: 1,
        consumer_failed: 0,
        consumer_active: false,
      },
    };

    queryClient.setQueryData(["config"], config);
    queryClient.setQueryData(["notification-status"], notifications);
    queryClient.setQueryData(["log-cleanup"], {
      enabled: false,
      retention_days: 30,
      last_run_at: null,
      next_run_at: null,
    });

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <ConfigPage />
      </QueryClientProvider>,
    );

    expect(markup).toContain("系统设置");
    const pageShell = markup.match(/<div[^>]*data-testid="system-settings-page"[^>]*>/)?.[0];
    expect(pageShell).toContain('class="w-full space-y-4"');
    expect(markup).not.toContain("max-w-7xl");
    expect(markup).toContain('data-testid="system-settings-flow"');
    expect(markup).toContain("grid items-start gap-4 xl:grid-cols-2");
    expect(markup).toContain('data-testid="system-settings-flow-primary"');
    expect(markup).toContain('data-testid="system-settings-flow-secondary"');
    expect(markup.match(/grid min-w-0 content-start gap-4/g)).toHaveLength(2);
    expect(markup).not.toContain("xl:row-span-2");
    expect(markup.match(/data-size="sm"/g)).toHaveLength(4);
    expect(markup).not.toContain("执行模式");
    expect(markup).toContain("Sub2API 连接");
    expect(markup).not.toContain("运行状态");
    expect(markup).not.toContain("自动巡检");
    expect(markup).not.toContain("真实流量采集");
    expect(markup).not.toContain("页面数据更新");
    expect(markup).not.toContain("上一轮概要");
    expect(markup).not.toContain('data-testid="last-inspection-summary"');
    expect(markup).toContain("已配置，留空则不修改");
    expect(markup).toContain("请求超时（秒）");
    expect(markup).toContain("保存并测试同步");
    expect(markup).toContain("账号创建默认值");
    expect(markup).toContain('aria-label="平台接入"');
    expect(markup).toContain('aria-label="默认值与数据维护"');
    expect(markup).toContain("默认并发");
    expect(markup).toContain("默认优先级");
    expect(markup).toContain("保存默认参数");
    expect(markup).not.toContain('data-testid="runtime-controls"');
    expect(markup).not.toContain("divide-border/70 divide-y rounded-lg border px-3");
    expect(markup).not.toContain("first:pt-0 last:pb-0");
    expect(markup).toContain("QQBot 通知接入");
    expect(markup).not.toContain("告警生产者队列");
    expect(markup).not.toContain("通知消费者队列");
    expect(markup).not.toContain('data-testid="notification-queues"');
    expect(markup).toContain('data-testid="notification-credentials"');
    expect(markup).toContain("grid items-start gap-x-5 gap-y-4 sm:grid-cols-2");
    expect(markup).toContain('data-testid="notification-destination"');
    expect(markup).toContain("sm:grid-cols-[minmax(10rem,0.7fr)_minmax(0,1.3fr)]");
    expect(markup).toContain("border-border/70 flex flex-wrap justify-end gap-2 border-t pt-4");
    expect(markup).toContain("打开 QQ 开放平台");
    expect(markup).toContain("开发设置");
    expect(markup).toContain('placeholder="输入 user_openid"');
    expect(markup).toContain("查看事件服务接入说明");
    expect(markup).toContain("连接获取");
    expect(markup).toContain("系统会自动填入 user_openid");
    expect(markup).toContain("机器人无需回复");
    expect(markup).not.toContain("本控制台目前只负责发送通知");
    expect(markup).toContain("https://q.qq.com/qqbot/#/developer/developer-setting");
    expect(markup).toContain(
      "https://bot.q.qq.com/wiki/develop/api-v2/dev-prepare/interface-framework/event-emit.html",
    );
    expect(markup).toContain('value="configured-app"');
    expect(markup).toContain('value="configured-target"');
    expect(markup.match(/placeholder="已配置，留空则不修改"/g)).toHaveLength(2);
    expect(markup).not.toContain("secret-value");
    expect(markup).toContain("日志保留");
    expect(markup).toContain("定时清理");
    expect(markup).toContain('aria-label="日志保留天数"');
    expect(markup).toContain("立即按期限清理");
    expect(markup.match(/flex items-start justify-between gap-3/g)).toHaveLength(2);
    const cleanupLabel = markup.match(
      /<label[^>]*for="([^"]+)"[^>]*>[\s\S]*?<span[^>]*>定时清理<\/span>/,
    );
    expect(cleanupLabel?.[1]).toBeTruthy();
    expect(markup).toContain(`id="${cleanupLabel?.[1]}"`);
    expect(markup).not.toContain("运行环境");
    expect(markup).not.toContain("目标盈利比例");
    expect(markup).not.toContain("全局默认策略");
    expect(markup).not.toContain("余额告警阈值");
    expect(markup).not.toContain("数据库状态");
    expect(markup).not.toContain("控制台初始化");
    expect(markup).not.toContain("数据目录");
    expect(markup).not.toContain("重新读取");
    expect(markup).not.toContain("主动探测已关闭");
    expect(markup).not.toContain("鉴权恢复配置");
    expect(markup).not.toContain("保存授权记录");
  });

  it("maps the persisted Sub2API address and timeout back into the form", () => {
    expect(
      targetFormFromConfig({
        admin_base_url: "https://sub2api.example.test",
        request_timeout_seconds: 60,
      }),
    ).toEqual({
      admin_base_url: "https://sub2api.example.test",
      admin_key: "",
      request_timeout_seconds: "60",
    });
  });

  it("maps public notification identifiers while keeping the secret input empty", () => {
    expect(
      notificationFormFromStatus({
        configured: true,
        app_id: "app-123",
        client_secret_configured: true,
        home_channel: "target-456",
        channel_type: "group",
        destination_configured: true,
        configuration_errors: [],
        queues: {
          producer_firing: 0,
          producer_recovered: 0,
          consumer_pending: 0,
          consumer_failed: 0,
          consumer_active: false,
        },
      }),
    ).toEqual({
      app_id: "app-123",
      client_secret: "",
      home_channel: "target-456",
      home_channel_type: "group",
    });
  });

  it("explains which QQ event identifier each notification target needs", () => {
    expect(notificationTargetField("c2c")).toEqual({
      placeholder: "输入 user_openid",
      description:
        "点击“连接获取”后，给机器人发送任意私聊消息，系统会自动填入 user_openid；机器人无需回复。",
    });
    expect(notificationTargetField("group")).toEqual({
      placeholder: "输入 group_openid",
      description:
        "点击“连接获取”后，在目标群里 @机器人并发送任意消息，系统会自动填入 group_openid；机器人无需回复。",
    });
    expect(notificationTargetField("channel")).toEqual({
      placeholder: "输入 channel_id",
      description:
        "点击“连接获取”后，在目标子频道里 @机器人并发送任意消息，系统会自动填入 channel_id；机器人无需回复。",
    });
  });

  it("accepts only a complete successful target discovery result", () => {
    const task: Task = {
      id: "qqbot-target-1",
      skill: "qqbot",
      operation: "discover-notification-target",
      status: "succeeded",
      progress: 100,
      message: "已获取通知目标并自动填入",
      result: {
        target_id: " user-open-id ",
        target_type: "c2c",
        event_type: "C2C_MESSAGE_CREATE",
        source_name: "测试用户",
        captured_at: "2026-08-29T10:00:00Z",
      },
      created_at: "2026-08-29T09:59:00Z",
      updated_at: "2026-08-29T10:00:00Z",
    };
    expect(notificationTargetResultFromTask(task)).toEqual({
      id: "user-open-id",
      type: "c2c",
      eventType: "C2C_MESSAGE_CREATE",
      sourceName: "测试用户",
      capturedAt: "2026-08-29T10:00:00Z",
    });
    expect(notificationTargetResultFromTask({ ...task, status: "waiting_input" })).toBeNull();
    expect(
      notificationTargetResultFromTask({ ...task, result: { target_type: "c2c" } }),
    ).toBeNull();
  });
});
