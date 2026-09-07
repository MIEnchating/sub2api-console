import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  ConfigPage,
  notificationFormFromStatus,
  notificationTargetResultFromTask,
  notificationTargetField,
  saveTargetWithOptionalSync,
  targetFormFromConfig,
} from "../App";
import type { AutoInspectionStatus, NotificationStatus, RuntimeConfig, Task } from "../api";
import type { ConfigTab } from "../features/config/constants";

const runtimeConfigFixture: RuntimeConfig = {
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

const notificationStatusFixture: NotificationStatus = {
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

const autoInspectionStatusFixture: AutoInspectionStatus = {
  enabled: true,
  interval_seconds: 15,
  running: false,
  monitoring_configured: true,
  monitoring_enabled: true,
  monitoring_checked_at: "2026-09-05T14:40:25Z",
  last_run_duration_ms: 29_681,
  last_summary: {
    channels: 233,
    probed: 10,
    samples: 112,
    fused: 2,
    recovered: 1,
    applied: 24,
    cleaned_up: 0,
    alerts: 3,
  },
  last_run_at: "2026-09-05T14:40:25Z",
  next_run_at: "2026-09-05T14:40:40Z",
  last_status: "succeeded",
  last_error: null,
  last_task_id: "inspection-1",
  queue: [],
  heartbeat_history: [],
};

function configuredQueryClient() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  });
  queryClient.setQueryData(["config"], runtimeConfigFixture);
  queryClient.setQueryData(["notification-status"], notificationStatusFixture);
  queryClient.setQueryData(["auto-inspection"], autoInspectionStatusFixture);
  queryClient.setQueryData(["account-creation-settings"], {
    default: {
      models: ["model-a"],
      concurrency: 10,
      load_factor: null,
      priority: 1,
      pool_mode: false,
      pool_mode_retry_count: 2,
      pool_mode_retry_status_codes: [429],
    },
    groups: [],
    platform_probe_models: {},
  });
  queryClient.setQueryData(["account-model-sync-settings"], {
    blocked_patterns: ["claude-*", "*-image-*"],
  });
  queryClient.setQueryData(["log-cleanup"], {
    enabled: false,
    retention_days: 30,
    last_run_at: null,
    next_run_at: null,
  });
  return queryClient;
}

function configMarkup(activeTab: ConfigTab) {
  return renderToStaticMarkup(
    <QueryClientProvider client={configuredQueryClient()}>
      <ConfigPage activeTab={activeTab} />
    </QueryClientProvider>,
  );
}

function InteractiveConfigPage() {
  const [activeTab, setActiveTab] = useState<ConfigTab>("connection");
  return <ConfigPage activeTab={activeTab} onTabChange={setActiveTab} />;
}

describe("系统设置页面职责", () => {
  it("默认只挂载连接设置，并提供适配移动端的四个页签", () => {
    const connectionMarkup = configMarkup("connection");

    expect(connectionMarkup).toContain("系统设置");
    expect(connectionMarkup).toContain('aria-label="刷新系统设置"');
    const pageShell = connectionMarkup.match(
      /<div[^>]*data-testid="system-settings-page"[^>]*>/,
    )?.[0];
    expect(pageShell).toContain("w-full");
    expect(pageShell).not.toContain("max-w-");
    expect(connectionMarkup).toContain('data-slot="page-content"');
    expect(connectionMarkup).toContain("min-h-0 flex-1 overflow-hidden");
    expect(connectionMarkup).toContain('role="tablist"');
    expect(connectionMarkup).toContain('aria-label="系统设置分类"');
    expect(connectionMarkup).toContain('data-testid="system-settings-tabs"');
    expect(connectionMarkup).toContain("sticky top-0");
    expect(connectionMarkup).toContain("grid w-full grid-cols-2 sm:grid-cols-4");
    expect(connectionMarkup).toContain("flex h-full min-h-0 w-full flex-col gap-4 overflow-hidden");
    expect(connectionMarkup.match(/role="tab"/g)).toHaveLength(4);
    expect(connectionMarkup).toContain("连接设置");
    expect(connectionMarkup).toContain("账号设置");
    expect(connectionMarkup).toContain("通知设置");
    expect(connectionMarkup).toContain("界面与日志");
    expect(connectionMarkup).toMatch(
      /<button(?=[^>]*id="config-tab-connection")(?=[^>]*aria-selected="true")[^>]*>/,
    );
    expect(connectionMarkup).toContain('id="config-panel-connection"');
    expect(connectionMarkup).toContain('aria-labelledby="config-tab-connection"');
    expect(connectionMarkup).toContain("overflow-y-auto overscroll-contain");
    expect(connectionMarkup).not.toContain("max-w-4xl");
    expect(connectionMarkup).toContain('data-testid="connection-settings-layout"');
    expect(connectionMarkup).toContain("xl:grid-cols-[minmax(0,1.45fr)_minmax(0,0.75fr)]");
    expect(connectionMarkup.match(/data-slot="card"[^>]*data-size="sm"/g)).toHaveLength(2);
    expect(connectionMarkup).toContain('data-testid="runtime-controls"');
    expect(connectionMarkup).toContain("Sub2API 连接");
    expect(connectionMarkup).toContain("执行模式");
    expect(connectionMarkup).toMatch(
      /<div(?=[^>]*data-testid="last-inspection-summary")(?=[^>]*class="[^"]*h-full[^"]*")[^>]*>/,
    );
    expect(connectionMarkup).toContain("上一轮概要");
    expect(connectionMarkup).toContain("执行时间：09/05 14:40:25");
    expect(connectionMarkup).toContain("执行成功");
    expect(connectionMarkup).toContain("29.7 秒");
    expect(connectionMarkup).toMatch(
      /<button(?=[^>]*aria-label="完全模式：)(?=[^>]*aria-pressed="true")[^>]*>/,
    );
    expect(connectionMarkup).not.toContain('data-testid="account-creation-settings-card"');
    expect(connectionMarkup).not.toContain("QQBot 通知接入");
    expect(connectionMarkup).not.toContain("菜单设置");
    expect(connectionMarkup).not.toContain("日志保留");
  });

  it("每个页签只挂载所属设置内容", () => {
    const accountMarkup = configMarkup("accounts");
    const notificationMarkup = configMarkup("notifications");
    const interfaceMarkup = configMarkup("interface");

    expect(accountMarkup).toContain('data-testid="account-creation-settings-card"');
    expect(accountMarkup).toContain('data-testid="model-sync-settings-card"');
    expect(accountMarkup).toContain('data-testid="account-settings-layout"');
    expect(accountMarkup).toContain("xl:grid-cols-[minmax(0,1.6fr)_minmax(0,0.7fr)]");
    expect(accountMarkup).toContain("全局屏蔽模型");
    expect(accountMarkup).toContain("claude-*");
    expect(accountMarkup).toContain("账号模型");
    expect(accountMarkup).toContain("并发上限");
    expect(accountMarkup).toContain("负载因子");
    expect(accountMarkup).toContain("池模式");
    expect(accountMarkup).not.toContain("Sub2API 连接");
    expect(accountMarkup).not.toContain("QQBot 通知接入");

    expect(notificationMarkup).toContain("QQBot 通知接入");
    expect(notificationMarkup).toContain('data-testid="notification-credentials"');
    expect(notificationMarkup).toContain('data-testid="notification-destination"');
    expect(notificationMarkup).toContain('value="configured-app"');
    expect(notificationMarkup).toContain('value="configured-target"');
    expect(notificationMarkup).not.toContain('data-testid="account-creation-settings-card"');
    expect(notificationMarkup).not.toContain("菜单设置");

    expect(interfaceMarkup).toContain("xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]");
    expect(interfaceMarkup).toContain("菜单设置");
    expect(interfaceMarkup).toContain("当前显示 23 / 23 个菜单入口");
    expect(interfaceMarkup).toContain('aria-label="在菜单中显示账号管理"');
    expect(interfaceMarkup).toContain('aria-label="系统设置说明"');
    expect(interfaceMarkup).not.toContain("/config · 始终显示");
    expect(interfaceMarkup).toContain("日志保留");
    expect(interfaceMarkup).toContain("定时清理");
    expect(interfaceMarkup).toContain('aria-label="定时清理说明"');
    expect(interfaceMarkup).not.toContain("每天检查并删除超过保留期的日志");
    expect(interfaceMarkup).toContain('aria-label="日志保留天数"');
    expect(interfaceMarkup).toContain("立即按期限清理");
    expect(interfaceMarkup).toContain("sm:grid-cols-2 lg:grid-cols-3");
    expect(interfaceMarkup.match(/data-slot="card"[^>]*data-size="sm"/g)).toHaveLength(2);
    expect(interfaceMarkup).not.toContain("Sub2API 连接");
    expect(interfaceMarkup).not.toContain("QQBot 通知接入");
    expect(notificationMarkup).not.toContain("max-w-5xl");
  });

  it("点击和方向键会切换页签及对应面板", async () => {
    const user = userEvent.setup();
    const queryClient = configuredQueryClient();
    render(
      <QueryClientProvider client={queryClient}>
        <InteractiveConfigPage />
      </QueryClientProvider>,
    );

    const connectionTab = screen.getByRole("tab", { name: "连接设置" });
    expect(connectionTab).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveAccessibleName("连接设置");
    expect(screen.getByText("Sub2API 连接")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "通知设置" }));
    expect(screen.getByRole("tab", { name: "通知设置" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveAccessibleName("通知设置");
    expect(screen.getByText("QQBot 通知接入")).toBeInTheDocument();
    expect(screen.queryByText("Sub2API 连接")).not.toBeInTheDocument();

    connectionTab.focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "账号设置" })).toHaveFocus();
    expect(screen.getByRole("tabpanel", { name: "账号设置" })).toBeInTheDocument();
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

  it("commits a saved management target before a follow-up sync fails to start", async () => {
    const queryClient = new QueryClient();
    const saved: RuntimeConfig = {
      database_available: true,
      data_database_available: true,
      mode: "完全模式",
      config_keys: [],
      secret_values_hidden: true,
      probes_enabled: true,
      account_default_concurrency: 10,
      account_default_priority: 1,
      admin_base_url: "https://saved.example.test",
      request_timeout_seconds: 45,
      initialized: true,
      target_configured: true,
      console_username: "admin",
      configuration_errors: [],
    };
    const steps: string[] = [];
    let formBaseline = targetFormFromConfig({
      admin_base_url: "https://old.example.test",
      request_timeout_seconds: 30,
    });
    const syncError = new Error("同步服务不可用");

    const outcome = await saveTargetWithOptionalSync({
      persist: async () => {
        steps.push("persist");
        return saved;
      },
      commit: (value) => {
        steps.push("commit");
        queryClient.setQueryData(["config"], value);
        formBaseline = targetFormFromConfig(value);
      },
      startSync: async () => {
        steps.push("sync");
        throw syncError;
      },
      syncAfterSave: true,
    });

    expect(steps).toEqual(["persist", "commit", "sync"]);
    expect(queryClient.getQueryData(["config"])).toBe(saved);
    expect(formBaseline).toEqual({
      admin_base_url: "https://saved.example.test",
      admin_key: "",
      request_timeout_seconds: "45",
    });
    expect(outcome).toEqual({ task: null, syncFailed: true, syncError });
  });

  it("keeps log cleanup controls disabled until its configuration is read successfully", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { enabled: false, retry: false } },
    });
    queryClient.setQueryData<RuntimeConfig>(["config"], {
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
    });
    await queryClient.prefetchQuery({
      queryKey: ["log-cleanup"],
      queryFn: async () => {
        throw new Error("读取失败");
      },
      retry: false,
    });

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={queryClient}>
        <ConfigPage activeTab="interface" />
      </QueryClientProvider>,
    );

    expect(markup).toContain("读取成功后才能修改设置或清理日志");
    expect(markup).toContain('aria-label="刷新日志清理配置"');
    expect(markup).toMatch(
      /<span(?=[^>]*role="switch")(?=[^>]*aria-label="定时清理")(?=[^>]*aria-disabled="true")[^>]*>/,
    );
    expect(markup).toMatch(/<input(?=[^>]*aria-label="日志保留天数")(?=[^>]*disabled)[^>]*>/);
    expect(markup).toMatch(
      /<button(?=[^>]*data-testid="log-cleanup-clear")(?=[^>]*disabled)[^>]*>/,
    );
    expect(markup).toMatch(/<button(?=[^>]*data-testid="log-cleanup-save")(?=[^>]*disabled)[^>]*>/);
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
