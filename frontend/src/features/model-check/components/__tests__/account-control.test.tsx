import { QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import type { AccountStatus, Task } from "@/api";
import { account } from "@/features/accounts/__tests__/fixtures";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(overrides: Partial<AccountStatus> = {}, rejectWrite = false) {
  let current = { ...account, platform: "openai", ...overrides };
  const requests: unknown[] = [];
  const task: Task = {
    id: "control-41",
    skill: "console",
    operation: "account-control",
    status: "running",
    progress: 0,
    message: "执行中",
    result: {},
    created_at: "2026-09-22T00:00:00Z",
    updated_at: "2026-09-22T00:00:00Z",
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/accounts/41") return Response.json(current);
      if (path === "/api/accounts") return Response.json([current]);
      if (path === "/api/accounts/41/control") {
        requests.push(JSON.parse(String(init?.body)));
        if (rejectWrite) return Response.json({ detail: "管理平台拒绝写入" }, { status: 403 });
        return Response.json(task);
      }
      if (path === "/api/tasks/control-41") return Response.json(task);
      throw new Error(`Unexpected request: ${path}`);
    }),
  );
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { staleTime: Infinity, retry: false } });
  client.setQueryData(["accounts"], [current]);
  client.setQueryData(["policy"], {});
  client.setQueryData(["model-animation", "history"], []);
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["terminal-continuity", "history"], []);
  for (const name of ["group", "platform"])
    client.setQueryData(["dictionaries", name], { items: [] });
  const view = render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return {
    client,
    requests,
    task,
    update: (changes: Partial<AccountStatus>): void => {
      current = { ...current, ...changes };
    },
    dispose: (): void => {
      view.unmount();
      client.clear();
    },
  };
}

it.each(["账号检测", "前置检测", "终端续接检测"])(
  "%s 卡片确认熔断后按稳定 ID 提交任务，完成后显示恢复入口",
  async (tab) => {
    const view = setup();
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: tab }));
    const panel = screen.getByRole("tabpanel", { name: tab });
    await user.click(await within(panel).findByRole("button", { name: "手动熔断" }));
    const dialog = await screen.findByRole("dialog", { name: "手动熔断" });
    expect(dialog).toHaveTextContent("ID：41");
    expect(view.requests).toEqual([]);
    const confirm = within(dialog).getByRole("button", { name: "确认手动熔断" });
    await waitFor(() => expect(confirm).toBeEnabled());
    await user.click(confirm);
    await waitFor(() => expect(view.requests).toEqual([{ action: "fuse" }]));
    expect(within(panel).getByRole("button", { name: "手动熔断" })).toBeDisabled();
    view.update({ health: "fused", routing_state: "fused", schedulable: false });
    view.task.status = "succeeded";
    view.task.message = "手动熔断完成";
    await act(async () => {
      await view.client.invalidateQueries({ queryKey: ["account-scheduling"] });
    });
    await waitFor(() =>
      expect(within(panel).getByRole("button", { name: "解除熔断" })).toBeEnabled(),
    );
    view.dispose();
  },
);

it.each([
  { health: "fused", paused: false, label: "解除熔断", action: "recover" },
  { health: "paused", paused: true, label: "恢复调度", action: "resume" },
])("$health 账号确认后提交 $action", async (scenario) => {
  const view = setup({ health: scenario.health, paused: scenario.paused, schedulable: false });
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: scenario.label }));
  const confirm = await screen.findByRole("button", { name: `确认${scenario.label}` });
  await waitFor(() => expect(confirm).toBeEnabled());
  await user.click(confirm);
  await waitFor(() => expect(view.requests).toEqual([{ action: scenario.action }]));
  view.dispose();
});

it.each([{ manual_priority: 1 }, { health: "excluded" }])(
  "受保护账号 %j 禁用熔断和恢复",
  (state) => {
    const view = setup(state);
    expect(screen.getByRole("button", { name: "手动熔断" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "恢复调度" })).toBeDisabled();
    expect(view.requests).toEqual([]);
    view.dispose();
  },
);

it("确认期间账号变为等待并发额度时禁止恢复", async () => {
  const view = setup({ schedulable: false });
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "恢复调度" }));
  const confirm = await screen.findByRole("button", { name: "确认恢复调度" });
  await waitFor(() => expect(confirm).toBeEnabled());
  view.update({ health: "concurrency_limited", routing_state: "concurrency_limited" });
  await act(async () => {
    await view.client.invalidateQueries({ queryKey: ["account-detail", "41"] });
  });
  await waitFor(() => expect(confirm).toBeDisabled());
  expect(view.requests).toEqual([]);
  view.dispose();
});

it("取消确认不写入，任务创建失败时保留确认并显示原因", async () => {
  const view = setup({}, true);
  const error = vi.spyOn(toast, "error");
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "手动熔断" }));
  await user.click(await screen.findByRole("button", { name: "取消" }));
  expect(view.requests).toEqual([]);
  await user.click(screen.getByRole("button", { name: "手动熔断" }));
  const confirm = await screen.findByRole("button", { name: "确认手动熔断" });
  await waitFor(() => expect(confirm).toBeEnabled());
  fireEvent.click(confirm);
  await waitFor(() => expect(error).toHaveBeenCalled());
  expect(screen.getByRole("dialog", { name: "手动熔断" })).toBeVisible();
  await waitFor(() => expect(confirm).toBeEnabled());
  view.dispose();
});

it("熔断任务执行时切换标签仍禁用重复操作，任务失败通过悬浮提示反馈", async () => {
  const view = setup();
  const error = vi.spyOn(toast, "error");
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "手动熔断" }));
  const confirm = await screen.findByRole("button", { name: "确认手动熔断" });
  await waitFor(() => expect(confirm).toBeEnabled());
  await user.click(confirm);
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await user.click(screen.getByRole("tab", { name: "终端续接检测" }));
  const panel = screen.getByRole("tabpanel", { name: "终端续接检测" });
  expect(await within(panel).findByRole("button", { name: "手动熔断" })).toBeDisabled();
  view.task.status = "failed";
  view.task.message = "上游写入失败，请重试";
  await act(async () => {
    await view.client.invalidateQueries({ queryKey: ["account-scheduling"] });
  });
  await waitFor(() =>
    expect(error).toHaveBeenCalledWith("上游写入失败，请重试", expect.any(Object)),
  );
  await waitFor(() =>
    expect(within(panel).getByRole("button", { name: "手动熔断" })).toBeEnabled(),
  );
  expect(within(panel).queryByText("上游写入失败，请重试")).not.toBeInTheDocument();
  view.dispose();
});

it("最新账号读取失败时禁止确认并可重试，返回其他账号 ID 时仍禁止写入", async () => {
  const view = setup();
  const network = vi.mocked(fetch);
  const original = network.getMockImplementation()!;
  network.mockImplementation(async (input, init) =>
    String(input) === "/api/accounts/41"
      ? Response.json({ detail: "账号读取失败，请重试" }, { status: 403 })
      : original(input, init),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "手动熔断" }));
  const confirm = await screen.findByRole("button", { name: "确认手动熔断" });
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(confirm).toBeDisabled();
  view.update({ id: "42" });
  network.mockImplementation(original);
  await user.click(retry);
  await waitFor(() => expect(screen.getByText("账号状态尚未确认")).toBeVisible());
  expect(confirm).toBeDisabled();
  expect(view.requests).toEqual([]);
  view.dispose();
});

it("键盘打开熔断确认后取消将焦点还给入口且不发起写入", async () => {
  const view = setup();
  const user = userEvent.setup();
  const button = screen.getByRole("button", { name: "手动熔断" });
  act(() => button.focus());
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("dialog", { name: "手动熔断" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "取消" }));
  await waitFor(() => expect(button).toHaveFocus());
  expect(view.requests).toEqual([]);
  view.dispose();
});
