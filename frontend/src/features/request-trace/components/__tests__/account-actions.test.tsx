import { Toaster, toast } from "sonner";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AccountDetail, Task, UsageRecord } from "@/api";
import { SystemLogSearchPanel } from "../system-log-search-panel";
import { TraceAccountActions } from "../trace-account-actions";

function account(overrides: Partial<AccountDetail> = {}): AccountDetail {
  return {
    id: "206",
    name: "极算云-0.15",
    groups: [],
    upstream_id: null,
    upstream_host: null,
    upstream_type: null,
    schedulable: true,
    priority: 1,
    load_factor: null,
    concurrency: 1,
    multiplier: null,
    balance: null,
    paused: false,
    paused_reason: null,
    routing_state: "healthy",
    health_status: null,
    health: "healthy",
    desired_health: null,
    apply_pending: false,
    apply_error: null,
    decision_state: null,
    decision_reason: null,
    failure_streak: 0,
    recovery_pass_streak: 0,
    target_priority: null,
    target_load_factor: null,
    target_schedulable: null,
    target_concurrency: null,
    health_score: null,
    short_score: null,
    long_score: null,
    sample_count: 0,
    recent_results: [],
    ttfb_p50_ms: null,
    ttfb_p95_ms: null,
    weight: null,
    metadata: {},
    group_rates: {},
    group_ids: {},
    bindings: [],
    test_models: [],
    ...overrides,
  };
}

function task(status: Task["status"] = "queued"): Task {
  return {
    id: "control-206",
    skill: "console",
    operation: "account-control",
    status,
    progress: status === "queued" ? 0 : 100,
    message: status === "failed" ? "上游写入失败" : "处置完成",
    result: {},
    created_at: "2026-09-07T00:00:00Z",
    updated_at: "2026-09-07T00:00:00Z",
  };
}

function mockNetwork(current = account(), completed = task("succeeded")) {
  const writes: Array<{ path: string; body: unknown }> = [];
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const path = String(input);
    if (path === "/api/accounts/206/control") {
      writes.push({ path, body: JSON.parse(String(init?.body)) });
      return Response.json(task());
    }
    if (path === "/api/accounts/206") return Response.json(current);
    if (path === "/api/tasks/control-206") return Response.json(completed);
    throw new Error(`Unexpected request: ${path}`);
  });
  vi.stubGlobal("fetch", fetch);
  return { fetch, writes };
}

const clients: QueryClient[] = [];
function renderActions(accountId = "206", panel = false): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      {panel ? <SystemLogSearchPanel /> : <TraceAccountActions accountId={accountId} />}
    </QueryClientProvider>,
  );
  return client;
}

afterEach(() => {
  toast.dismiss();
  for (const client of clients) client.clear();
  clients.length = 0;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("请求查询账号处置", () => {
  it("账号状态加载时保留可换行的操作区并禁用处置", () => {
    mockNetwork();
    renderActions();
    expect(screen.getByRole("status")).toHaveTextContent("正在读取账号状态");
    expect(screen.getByRole("group", { name: "账号 206 处置" })).toHaveClass("flex-wrap");
    expect(screen.getByRole("button", { name: "手动熔断" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "恢复调度" })).toBeDisabled();
  });

  it("查询命中日志中的稳定账号 ID 后展示账号处置", async () => {
    const network = mockNetwork();
    const fallback = network.fetch.getMockImplementation()!;
    const record: UsageRecord = {
      id: 1,
      request_id: "req-206",
      account_id: null,
      account_name: "旧账号名称",
      group_name: null,
      is_error: false,
      error_reason: null,
      duration_ms: null,
      first_token_ms: null,
      summary: "http request completed acc=206",
      observed_at: null,
      source: "system-log",
      payload: {},
    };
    network.fetch.mockImplementation(async (input, init) => {
      if (String(input).startsWith("/api/ops/system-logs?")) {
        return Response.json({ items: [record], page: 1, page_size: 20, total: 1 });
      }
      return fallback(input, init);
    });
    renderActions("206", true);
    fireEvent.change(screen.getByRole("textbox", { name: "request_id" }), {
      target: { value: "req-206" },
    });
    fireEvent.click(screen.getByRole("button", { name: "查询" }));
    expect(await screen.findByRole("group", { name: "账号 206 处置" })).toBeVisible();
    await waitFor(() => expect(screen.getByRole("button", { name: "手动熔断" })).toBeEnabled());
  });

  it("手动熔断先确认最新账号名称和 ID，确认后发送处置并刷新账号", async () => {
    const user = userEvent.setup();
    const network = mockNetwork();
    const client = renderActions();
    client.setQueryData(["accounts"], []);
    await waitFor(() => expect(screen.getByRole("button", { name: "手动熔断" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "手动熔断" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/极算云-0.15（ID：206）/)).toBeVisible();
    expect(network.writes).toEqual([]);
    await user.click(within(dialog).getByRole("button", { name: "确认手动熔断" }));
    await waitFor(() =>
      expect(network.writes).toEqual([
        { path: "/api/accounts/206/control", body: { action: "fuse" } },
      ]),
    );
    await waitFor(() => expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(true));
    expect(await screen.findByRole("status")).toHaveTextContent("处置完成");
  });

  it.each([
    { state: "fused", paused: false, label: "解除熔断", action: "recover" },
    { state: "paused", paused: true, label: "恢复调度", action: "resume" },
  ])("$state 账号确认恢复后提交 $action", async ({ state, paused, label, action }) => {
    const network = mockNetwork(account({ health: state, paused, schedulable: false }));
    const user = userEvent.setup();
    renderActions();
    await waitFor(() => expect(screen.getByRole("button", { name: label })).toBeEnabled());
    expect(screen.getByRole("button", { name: "手动熔断" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: label }));
    await user.click(screen.getByRole("button", { name: `确认${label}` }));
    await waitFor(() => expect(network.writes[0]?.body).toEqual({ action }));
  });

  it("任务执行中禁用处置，失败后显示失败原因", async () => {
    const network = mockNetwork(account(), task("running"));
    const client = renderActions();
    const user = userEvent.setup();
    await waitFor(() => expect(screen.getByRole("button", { name: "手动熔断" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "手动熔断" }));
    await user.click(screen.getByRole("button", { name: "确认手动熔断" }));
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("账号处置执行中"));
    expect(screen.getByRole("button", { name: "手动熔断" })).toBeDisabled();
    const fallback = network.fetch.getMockImplementation()!;
    network.fetch.mockImplementation(async (input, init) =>
      String(input).startsWith("/api/tasks/")
        ? Response.json(task("failed"))
        : fallback(input, init),
    );
    await client.invalidateQueries({ queryKey: ["account-scheduling"] });
    await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("上游写入失败"));
  });

  it("账号状态读取失败时禁用操作并支持重试", async () => {
    const network = mockNetwork();
    network.fetch.mockRejectedValueOnce(new Error("账号已删除，请同步账号"));
    renderActions();
    expect(await screen.findByText("账号已删除，请同步账号")).toBeVisible();
    expect(screen.getByRole("group", { name: "账号 206 处置" })).not.toHaveTextContent(
      "账号已删除，请同步账号",
    );
    expect(screen.getByRole("button", { name: "手动熔断" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "手动熔断" })).toBeEnabled());
  });

  it.each(["", "0", "account-name"])("账号 ID 为 %s 时不请求账号也不提供操作", (id) => {
    const network = mockNetwork();
    renderActions(id);
    expect(screen.getByText("账号 ID 未记录或无效，无法处置")).toBeVisible();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(network.fetch).not.toHaveBeenCalled();
  });

  it("人工优先位账号禁止熔断和恢复", async () => {
    mockNetwork(account({ manual_priority: 1 }));
    renderActions();
    expect(await screen.findByText("请先取消人工优先位")).toBeVisible();
    expect(screen.getByRole("button", { name: "手动熔断" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "恢复调度" })).toBeDisabled();
  });

  it("键盘打开确认框后取消不会发送熔断并将焦点还给入口", async () => {
    const network = mockNetwork();
    const user = userEvent.setup();
    renderActions();
    const button = screen.getByRole("button", { name: "手动熔断" });
    await waitFor(() => expect(button).toBeEnabled());
    await user.tab();
    expect(button).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(screen.getByRole("dialog", { name: "手动熔断" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "取消" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(network.writes).toEqual([]);
    await waitFor(() => expect(button).toHaveFocus());
  });

  it("创建熔断任务失败后保留确认框并允许重试", async () => {
    const network = mockNetwork();
    const fallback = network.fetch.getMockImplementation()!;
    network.fetch.mockImplementation(async (input, init) =>
      String(input).endsWith("/control")
        ? Response.json({ error: "账号处置启动失败，请稍后重试" }, { status: 503 })
        : fallback(input, init),
    );
    const user = userEvent.setup();
    renderActions();
    await waitFor(() => expect(screen.getByRole("button", { name: "手动熔断" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "手动熔断" }));
    await user.click(screen.getByRole("button", { name: "确认手动熔断" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "确认手动熔断" })).toBeEnabled());
    expect(screen.getByRole("dialog", { name: "手动熔断" })).toBeVisible();
    network.fetch.mockImplementation(fallback);
    await user.click(screen.getByRole("button", { name: "确认手动熔断" }));
    await waitFor(() => expect(network.writes).toHaveLength(1));
  });
});
