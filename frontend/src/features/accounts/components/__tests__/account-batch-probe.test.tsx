import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AccountSelectionToolbar, AccountsPage } from "@/App";
import { api, type AccountStatus, type Task, type TaskSummary } from "@/api";
import { AccountBatchProbeDialog } from "../account-batch-probe-dialog";

function account(id: string, manualPriority: number | null = null): AccountStatus {
  return {
    id,
    name: `账号 ${id}`,
    groups: ["codex"],
    upstream_id: null,
    upstream_host: null,
    upstream_type: "apikey",
    schedulable: true,
    priority: 1,
    load_factor: null,
    concurrency: 1,
    multiplier: "0.1",
    balance: null,
    paused: false,
    paused_reason: null,
    routing_state: "healthy",
    health_status: "healthy",
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
    health_score: 100,
    short_score: 100,
    long_score: 100,
    sample_count: 1,
    recent_results: [],
    ttfb_p50_ms: null,
    ttfb_p95_ms: null,
    weight: null,
    manual_priority: manualPriority,
  };
}

function task(status: Task["status"] = "queued"): Task {
  return {
    id: "batch-probe-1",
    skill: "sub2api-connectivity-test",
    operation: "active-probe",
    status,
    progress: status === "queued" ? 0 : 100,
    message: "探活完成：通过 2",
    result: {},
    created_at: "2026-09-07T00:00:00Z",
    updated_at: "2026-09-07T00:00:00Z",
  };
}

const clients: QueryClient[] = [];
function client(): QueryClient {
  const value = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(value);
  return value;
}

afterEach(() => {
  for (const value of clients) value.clear();
  clients.length = 0;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderDialog(accounts: AccountStatus[]) {
  const queryClient = client();
  const onOpenChange = vi.fn();
  const onStarted = vi.fn();
  const onPendingChange = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <AccountBatchProbeDialog
        open
        accounts={accounts}
        onOpenChange={onOpenChange}
        onStarted={onStarted}
        onPendingChange={onPendingChange}
      />
    </QueryClientProvider>,
  );
  return { queryClient, onOpenChange, onStarted, onPendingChange };
}

describe("账号批量探活", () => {
  it("在账号页勾选账号后通过批量探活入口确认准确的处理范围", async () => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    vi.spyOn(api, "accounts").mockResolvedValue([account("41"), account("42")]);
    vi.spyOn(api, "policy").mockReturnValue(new Promise(() => {}));
    const run = vi.spyOn(api, "runActiveProbe").mockResolvedValue(task());
    vi.spyOn(api, "task").mockResolvedValue(task("running"));
    render(
      <QueryClientProvider client={client()}>
        <AccountsPage />
      </QueryClientProvider>,
    );
    fireEvent.click(await screen.findByRole("checkbox", { name: "选择账号 账号 41（#41）" }));
    fireEvent.click(screen.getByRole("button", { name: "探活已选择的 1 个账号" }));
    const list = screen.getByRole("list", { name: "本次探活账号" });
    expect(within(list).getByText("ID 41")).toBeVisible();
    expect(within(list).queryByText("ID 42")).toBeNull();
    expect(run).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "确认探活" }));
    await waitFor(() => expect(run).toHaveBeenCalledWith({ account_ids: ["41"] }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "批量探活" })).toBeNull());
  });

  it("多选确认后只提交非人工优先账号，任务完成刷新账号并登记后台任务", async () => {
    const run = vi.spyOn(api, "runActiveProbe").mockResolvedValue(task());
    vi.spyOn(api, "task").mockResolvedValue(task("succeeded"));
    const view = renderDialog([account("41"), account("42"), account("43", 1)]);
    view.queryClient.setQueryData(["accounts"], []);
    expect(screen.getByText(/已跳过 1 个人工优先位账号/)).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "确认探活" }));
    await waitFor(() => expect(run).toHaveBeenCalledWith({ account_ids: ["41", "42"] }));
    await waitFor(() =>
      expect(view.queryClient.getQueryState(["accounts"])?.isInvalidated).toBe(true),
    );
    expect(view.queryClient.getQueryData<TaskSummary[]>(["tasks"])?.[0]).toMatchObject({
      id: "batch-probe-1",
      system_info: true,
    });
    expect(view.onOpenChange).toHaveBeenCalledWith(false);
    expect(view.onStarted).toHaveBeenCalled();
  });

  it("创建任务失败保留确认范围并允许重试", async () => {
    const run = vi
      .spyOn(api, "runActiveProbe")
      .mockRejectedValueOnce(new Error("账号 41 已删除，请刷新"));
    const view = renderDialog([account("41")]);
    fireEvent.click(screen.getByRole("button", { name: "确认探活" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("账号 41 已删除，请刷新");
    expect(view.onStarted).not.toHaveBeenCalled();
    expect(view.onOpenChange).not.toHaveBeenCalled();
    run.mockResolvedValue(task());
    vi.spyOn(api, "task").mockResolvedValue(task("running"));
    fireEvent.click(screen.getByRole("button", { name: "确认探活" }));
    await waitFor(() => expect(view.onStarted).toHaveBeenCalled());
  });

  it("启动未完成时禁用提交和关闭，避免重复创建任务", async () => {
    let resolve!: (value: Task) => void;
    vi.spyOn(api, "runActiveProbe").mockReturnValue(
      new Promise<Task>((done) => {
        resolve = done;
      }),
    );
    vi.spyOn(api, "task").mockResolvedValue(task("running"));
    const view = renderDialog([account("41")]);
    fireEvent.click(screen.getByRole("button", { name: "确认探活" }));
    expect(await screen.findByRole("button", { name: "正在启动" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "关闭" })).toBeDisabled();
    expect(view.onPendingChange).toHaveBeenCalledWith(true);
    resolve(task());
    await waitFor(() => expect(view.onStarted).toHaveBeenCalled());
  });

  it.each([{ accounts: [] }, { accounts: [account("41", 1)] }])(
    "没有可探活账号时禁止提交",
    ({ accounts }) => {
      renderDialog(accounts);
      expect(screen.getByRole("button", { name: "确认探活" })).toBeDisabled();
      expect(screen.getByRole("alert")).toHaveTextContent("没有可探活账号");
    },
  );

  it("超过 100 个账号时提示范围上限并禁止提交", () => {
    renderDialog(Array.from({ length: 101 }, (_, index) => account(String(index + 1))));
    expect(screen.getByRole("alert")).toHaveTextContent("单次最多探活 100 个账号");
    expect(screen.getByRole("button", { name: "确认探活" })).toBeDisabled();
  });

  it("批量操作忙碌时禁用探活按钮", () => {
    render(
      <AccountSelectionToolbar
        selectedCount={2}
        pending
        onClear={vi.fn()}
        onSyncModels={vi.fn()}
        onProbe={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "探活已选择的 2 个账号" })).toBeDisabled();
  });
});
