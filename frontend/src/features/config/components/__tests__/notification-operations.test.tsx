import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Toaster, toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfigPage } from "@/App";
import type { NotificationStatus, RuntimeConfig } from "@/api";

const clients: QueryClient[] = [];

afterEach(() => {
  toast.dismiss();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function renderNotifications(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  });
  clients.push(client);
  const config: RuntimeConfig = {
    database_available: true,
    data_database_available: true,
    mode: "完全模式",
    config_keys: [],
    secret_values_hidden: true,
    probes_enabled: true,
    account_default_concurrency: 10,
    account_default_priority: 1,
    admin_base_url: "https://sub2api.example.test",
    request_timeout_seconds: 30,
    initialized: true,
    target_configured: true,
    console_username: "admin",
    configuration_errors: [],
  };
  const notifications: NotificationStatus = {
    configured: true,
    app_id: "bot-app",
    client_secret_configured: true,
    home_channel: "user-41",
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
  };
  client.setQueryData(["config"], config);
  client.setQueryData(["notification-status"], notifications);
  render(
    <QueryClientProvider client={client}>
      <ConfigPage activeTab="notifications" />
      <Toaster
        icons={{
          error: <span role="img" aria-label="失败通知" />,
          success: <span role="img" aria-label="成功通知" />,
        }}
      />
    </QueryClientProvider>,
  );
}

describe("通知测试业务结果", () => {
  it.each([
    [false, "QQBot 拒绝发送：目标不可达", "QQBot 拒绝发送：目标不可达"],
    [false, "", "测试通知发送失败，请检查 QQBot 配置后重试"],
    [undefined, undefined, "测试通知发送失败，请检查 QQBot 配置后重试"],
  ] as const)("sent=%s 且 detail=%s 时展示失败原因而非成功状态", async (sent, detail, expected) => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              sent,
              detail,
              persisted: true,
              runtime_event_id: 1,
            }),
          ),
      ),
    );
    renderNotifications();

    await user.click(screen.getByRole("button", { name: "测试通知" }));

    expect(await screen.findByRole("img", { name: "失败通知" })).toBeInTheDocument();
    expect(screen.getByText(expected)).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "成功通知" })).not.toBeInTheDocument();
  });

  it("明确发送成功时展示服务端详情与成功状态", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              sent: true,
              detail: "已投递到指定会话",
              persisted: true,
              runtime_event_id: 2,
            }),
          ),
      ),
    );
    renderNotifications();

    await user.click(screen.getByRole("button", { name: "测试通知" }));

    expect(await screen.findByRole("img", { name: "成功通知" })).toBeInTheDocument();
    expect(screen.getByText("已投递到指定会话")).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "失败通知" })).not.toBeInTheDocument();
  });
});

it("目标发现启动期间锁定连接参数，启动失败后恢复编辑", async () => {
  const user = userEvent.setup();
  let finish!: (value: Response) => void;
  const starting = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(() => starting),
  );
  renderNotifications();

  await user.click(screen.getByRole("button", { name: "连接获取" }));
  expect(await screen.findByText("正在创建 QQBot 连接任务")).toBeInTheDocument();

  expect(screen.getByPlaceholderText("输入 App ID")).toBeDisabled();
  expect(screen.getByDisplayValue("user-41")).toBeDisabled();
  expect(screen.getByRole("combobox")).toBeDisabled();
  expect(screen.getByRole("button", { name: "保存通知设置" })).toBeDisabled();
  await act(async () =>
    finish(
      new Response(JSON.stringify({ error: { code: "start_failed", message: "连接启动失败" } }), {
        status: 502,
      }),
    ),
  );

  await waitFor(() => expect(screen.getByRole("button", { name: "连接获取" })).toBeEnabled());
  expect(screen.getByPlaceholderText("输入 App ID")).toBeEnabled();
  expect(screen.getByDisplayValue("user-41")).toBeEnabled();
});
