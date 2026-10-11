import { onlineManager, QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast, Toaster } from "sonner";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "@/api";
import { TaskCancelButton } from "../task-startup-state";

const clients: QueryClient[] = [];
afterEach(() => {
  act(() => {
    toast.dismiss();
  });
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

function setup(options: { hold?: boolean; fail?: boolean; compact?: boolean } = {}) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: 2, retryDelay: 0 },
    },
  });
  clients.push(client);
  const requests: string[] = [];
  let fail = options.fail;
  let accepted = true;
  let release: () => void = () => undefined;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      requests.push(`${init?.method ?? "GET"} ${path}`);
      if (init?.method === "DELETE") {
        if (options.hold) await gate;
        if (fail)
          return Response.json(
            { code: "task_conflict", detail: "任务已经结束，无法取消" },
            { status: 409 },
          );
        return Response.json({ cancelled: accepted });
      }
      if (path === "/api/tasks?limit=20") return Response.json([]);
      if (path.startsWith("/api/tasks/"))
        return Response.json({ id: path.split("/").at(-1), status: "cancelled" });
      throw new Error(`Unexpected request ${path}`);
    }),
  );
  const view = (id: string) => (
    <QueryClientProvider client={client}>
      <TaskCancelButton taskId={id} compact={options.compact} />
      <Toaster />
    </QueryClientProvider>
  );
  const rendered = render(view("first"));
  return {
    client,
    requests,
    release,
    recover: () => {
      fail = false;
    },
    refuse: () => {
      accepted = false;
    },
    switchTask: () => rendered.rerender(view("second")),
    unmount: rendered.unmount,
  };
}

it("取消成功后切换任务，新任务仍可取消", async () => {
  const state = setup();
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  expect(await screen.findByRole("button", { name: "已请求取消" })).toBeDisabled();
  state.switchTask();
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  await waitFor(() => expect(state.requests).toContain("DELETE /api/tasks/second"));
});

it("旧任务取消迟到时只失效旧任务缓存，不锁住新任务", async () => {
  const state = setup({ hold: true });
  state.client.setQueryData(["task", "first"], { id: "first" });
  state.client.setQueryData(["task", "second"], { id: "second" });
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  state.switchTask();
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.client.getQueryState(["task", "first"])?.isInvalidated).toBe(true);
  expect(state.client.getQueryState(["task", "second"])?.isInvalidated).toBe(false);
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
});

it("取消失败不继承自动重试，用户可再次提交", async () => {
  const state = setup({ fail: true });
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  await screen.findByText("任务已经结束，无法取消");
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.requests).toEqual(["DELETE /api/tasks/first"]);
  state.recover();
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  expect(await screen.findByRole("button", { name: "已请求取消" })).toBeDisabled();
  expect(state.requests).toEqual(["DELETE /api/tasks/first", "DELETE /api/tasks/first"]);
});

it("浏览器报告离线时立即尝试取消，不排队到恢复联网后执行", async () => {
  const state = setup();
  onlineManager.setOnline(false);
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  await waitFor(() => expect(state.requests).toEqual(["DELETE /api/tasks/first"]));
});

it("同一轮连续点击只发送一次取消，等待时按钮禁用", async () => {
  const state = setup({ hold: true });
  const button = screen.getByRole("button", { name: "取消任务" });
  act(() => {
    fireEvent.click(button);
    fireEvent.click(button);
  });
  expect(await screen.findByRole("button", { name: "取消中" })).toBeDisabled();
  expect(state.requests).toEqual(["DELETE /api/tasks/first"]);
  await act(async () => state.release());
  expect(await screen.findByRole("button", { name: "已请求取消" })).toBeDisabled();
});

it("旧任务取消失败迟到时不向新任务弹出过期错误", async () => {
  const state = setup({ hold: true, fail: true });
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  state.switchTask();
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(screen.queryByText("任务已经结束，无法取消")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
});

it("接口未确认取消时保留重试入口并提示核对状态", async () => {
  const state = setup();
  state.refuse();
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  await screen.findByText("服务未确认取消，请刷新任务状态后重试");
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
});

it("取消成功刷新任务列表，但不重新读取已关闭的任务详情", async () => {
  const state = setup();
  await state.client.fetchQuery({ queryKey: ["task", "first"], queryFn: () => api.task("first") });
  state.client.setQueryData(["tasks"], []);
  state.requests.length = 0;
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  await screen.findByRole("button", { name: "已请求取消" });
  expect(state.requests).toEqual(["DELETE /api/tasks/first"]);
  expect(state.client.getQueryState(["task", "first"])?.isInvalidated).toBe(true);
  expect(state.client.getQueryState(["tasks"])?.isInvalidated).toBe(true);
});

function ActiveTask(): React.JSX.Element {
  const task = useQuery({ queryKey: ["task", "first"], queryFn: () => api.task("first") });
  return <div>{task.data?.status}</div>;
}

function ActiveList(): React.JSX.Element {
  const tasks = useQuery({ queryKey: ["tasks"], queryFn: () => api.tasks() });
  return <div>任务数量：{tasks.data?.length}</div>;
}

it("取消成功后刷新正在展示的列表和任务详情，保留其他任务缓存", async () => {
  const state = setup();
  state.client.setQueryData(["task", "first"], { id: "first", status: "running" });
  state.client.setQueryData(["tasks"], [{ id: "first" }]);
  state.client.setQueryData(["task", "second"], { id: "second" });
  render(
    <QueryClientProvider client={state.client}>
      <ActiveTask />
      <ActiveList />
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  await screen.findByText("cancelled");
  await screen.findByText("任务数量：0");
  expect(state.client.getQueryState(["task", "second"])?.isInvalidated).toBe(false);
});

it("窗口关闭后取消成功仍标记缓存过期，不重新读取关闭的详情", async () => {
  const state = setup({ hold: true });
  await state.client.fetchQuery({ queryKey: ["task", "first"], queryFn: () => api.task("first") });
  state.client.setQueryData(["tasks"], []);
  state.requests.length = 0;
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  state.unmount();
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.client.getQueryState(["task", "first"])?.isInvalidated).toBe(true);
  expect(state.client.getQueryState(["tasks"])?.isInvalidated).toBe(true);
  expect(state.requests).toEqual(["DELETE /api/tasks/first"]);
});

it("取消遇到任务已结束时刷新正在展示的详情", async () => {
  const state = setup({ fail: true });
  state.client.setQueryData(["task", "first"], { id: "first", status: "running" });
  render(
    <QueryClientProvider client={state.client}>
      <ActiveTask />
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByRole("button", { name: "取消任务" }));
  await screen.findByText("cancelled");
  expect(state.requests).toContain("GET /api/tasks/first");
});

it("图标取消按钮具有可访问名称并支持键盘提交", async () => {
  const state = setup({ compact: true });
  await userEvent.tab();
  expect(screen.getByRole("button", { name: "取消任务" })).toHaveFocus();
  await userEvent.keyboard("{Enter}");
  expect(await screen.findByRole("button", { name: "已请求取消" })).toBeDisabled();
  expect(state.requests).toEqual(["DELETE /api/tasks/first"]);
});
