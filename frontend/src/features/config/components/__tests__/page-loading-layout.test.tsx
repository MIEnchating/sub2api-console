import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, within, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ConfigPage } from "@/App";
import type { ConfigTab } from "../../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function renderLoading(tab: ConfigTab): void {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ConfigPage activeTab={tab} />
    </QueryClientProvider>,
  );
}

it("连接配置首次读取时保留连接与概要的不等宽分栏，配置未就绪不显示保存入口", () => {
  renderLoading("connection");
  expect(screen.getByTestId("connection-settings-layout")).toHaveClass(
    "xl:grid-cols-[minmax(0,1.45fr)_minmax(0,0.75fr)]",
  );
  expect(screen.getByText("Sub2API 连接")).toBeVisible();
  expect(screen.getByTestId("last-inspection-summary")).toBeVisible();
  expect(screen.queryByRole("button", { name: "保存连接" })).not.toBeInTheDocument();
});

it("账号设置在父配置读取前就加载两个实际卡片，父配置返回后继续保留同一布局", async () => {
  renderLoading("accounts");
  const layout = screen.getByTestId("account-settings-layout");
  expect(layout).toHaveClass("xl:grid-cols-[minmax(0,1.6fr)_minmax(0,0.7fr)]");
  const account = screen.getByRole("status", { name: "正在读取账号设置" });
  const models = screen.getByRole("status", { name: "正在读取全局屏蔽模型" });
  await act(async () => {
    client.setQueryData(["config"], {
      account_default_concurrency: 10,
      account_default_priority: 1,
    });
  });
  expect(screen.getByTestId("account-settings-layout")).toBe(layout);
  expect(screen.getByRole("status", { name: "正在读取账号设置" })).toBe(account);
  expect(screen.getByRole("status", { name: "正在读取全局屏蔽模型" })).toBe(models);
  expect(screen.queryByRole("button", { name: /保存/ })).not.toBeInTheDocument();
});

it("通知配置首次读取只显示一个卡片，并沿用凭据和目标的字段分栏", () => {
  renderLoading("notifications");
  const panel = screen.getByTestId("system-settings-panel");
  expect(panel.querySelectorAll('[data-slot="card"]')).toHaveLength(1);
  const loading = within(panel).getByRole("status", { name: "正在读取通知设置" });
  expect(loading).toHaveTextContent("QQBot 通知接入");
  expect(screen.getByTestId("notification-credentials")).toHaveClass("sm:grid-cols-2");
  expect(screen.getByTestId("notification-destination")).toHaveClass(
    "sm:grid-cols-[minmax(10rem,0.7fr)_minmax(0,1.3fr)]",
  );
});

it("界面与日志首次读取保留本地菜单和不等宽分栏，日志区域显示骨架", () => {
  renderLoading("interface");
  expect(screen.getByTestId("system-settings-panel")).toHaveClass(
    "xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]",
  );
  expect(screen.getByTestId("navigation-settings-card")).toBeVisible();
  expect(screen.getByRole("status", { name: "正在读取日志保留设置" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(screen.queryByRole("button", { name: "保存日志设置" })).not.toBeInTheDocument();
});

it("字典首次读取直接显示字典分类和列表骨架，父配置返回不替换成表单", async () => {
  renderLoading("dictionaries");
  const tabs = screen.getByRole("tablist", { name: "字典类型" });
  expect(
    screen.getByTestId("system-settings-panel").querySelectorAll('[data-slot="card"]'),
  ).toHaveLength(1);
  expect(screen.getAllByRole("row", { name: "正在读取字典" })).toHaveLength(4);
  await act(async () => {
    client.setQueryData(["config"], { mode: "监控模式" });
  });
  expect(screen.getByRole("tablist", { name: "字典类型" })).toBe(tabs);
  expect(screen.getAllByRole("row", { name: "正在读取字典" })).toHaveLength(4);
});

it("字典已加载时无关运行配置读取失败，字典内容和分类仍可使用", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith("/api/config"))
        return Response.json({ detail: "运行配置暂不可用" }, { status: 503 });
      return new Promise<Response>(() => {});
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["dictionaries", "platform"], {
    items: [
      {
        id: "openai",
        kind: "platform",
        value: "openai",
        name: "OpenAI",
        description: "平台",
        enabled: true,
        sort_order: 1,
      },
    ],
  });
  render(
    <QueryClientProvider client={client}>
      <ConfigPage activeTab="dictionaries" />
    </QueryClientProvider>,
  );
  await act(async () => {
    await client.refetchQueries({ queryKey: ["config"], exact: true });
  });
  await waitFor(() => expect(client.getQueryState(["config"])?.status).toBe("error"));
  expect(screen.getByText("OpenAI", { exact: true })).toBeVisible();
  expect(screen.getByRole("tablist", { name: "字典类型" })).toBeVisible();
});

it("上一轮概要后台读取失败时保留已读取统计", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ detail: "概要暂不可用" }, { status: 503 })),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["config"], { mode: "完全模式", target_configured: true });
  client.setQueryData(["notification-status"], {});
  client.setQueryData(["log-cleanup"], {});
  client.setQueryData(["auto-inspection"], {
    last_run_at: "2026-09-01T08:00:00Z",
    last_status: "succeeded",
    last_run_duration_ms: 5000,
    last_summary: {
      channels: 7,
      probed: 3,
      samples: 12,
      fused: 0,
      recovered: 1,
      applied: 2,
      cleaned_up: 0,
      alerts: 0,
    },
  });
  render(
    <QueryClientProvider client={client}>
      <ConfigPage activeTab="connection" />
    </QueryClientProvider>,
  );
  expect(screen.getByTestId("inspection-summary-grid")).toHaveTextContent("受管账号7");
  await act(async () => {
    await client.refetchQueries({ queryKey: ["auto-inspection"], exact: true });
  });
  await waitFor(() => expect(client.getQueryState(["auto-inspection"])?.status).toBe("error"));
  expect(screen.getByTestId("inspection-summary-grid")).toHaveTextContent("受管账号7");
});
