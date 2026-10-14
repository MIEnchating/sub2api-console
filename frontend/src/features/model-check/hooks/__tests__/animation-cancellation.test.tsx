import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationRequest, Task } from "@/api";
import { useAnimationTasks } from "../use-animation-tasks";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function runningTask(id: string, operation: string, accountID: string): Task {
  return {
    id,
    skill: "sub2api-model-animation",
    operation,
    status: "running",
    progress: 0,
    message: "检测中",
    result: { account_ids: [accountID] },
    created_at: "2026-10-14T00:00:00Z",
    updated_at: "2026-10-14T00:00:00Z",
  };
}

function setup(history: Task[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  const records = new Map(history.map((task) => [task.id, task]));
  const deleted: string[] = [];
  client.setQueryData(["model-animation", "history"], history);
  for (const task of history) client.setQueryData(["model-animation", "task", task.id], task);
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const url = String(input);
      if (init?.method === "DELETE") {
        const id = url.split("/").at(-1)!;
        deleted.push(id);
        const task = records.get(id)!;
        records.set(id, { ...task, status: "cancelled" });
        return Response.json({ cancelled: true });
      }
      if (init?.method === "POST") {
        const request = JSON.parse(String(init.body)) as AnimationRequest;
        const accountID = request.targets[0]?.account_id ?? "custom";
        const task = runningTask(
          `local-${accountID}`,
          request.mode === "precheck" ? "account-model-precheck" : "account-model-animation",
          accountID,
        );
        records.set(task.id, task);
        return Response.json(task);
      }
      if (url === "/api/model-checks/animations") return Response.json([...records.values()]);
      return Response.json(records.get(url.split("/").at(-1)!));
    }),
  );
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  return { client, view, deleted };
}

it("只有后台分组和历史检测运行时，页面取消不发送任何取消请求且保留账号忙碌状态", async () => {
  const fixture = setup([
    runningTask("managed", "managed-model-detection", "41"),
    runningTask("scheduled", "account-model-combined", "42"),
    runningTask("other-page", "account-model-animation", "43"),
  ]);
  await act(async () => fixture.view.result.current.cancelActive());
  expect(fixture.deleted).toEqual([]);
  expect(fixture.view.result.current.busyIDs).toEqual(new Set(["41", "42", "43"]));
});

it.each(["animation", "precheck"] as const)(
  "本页任务进入历史后，取消 %s 只影响本页对应模式且保留后台与自定义任务",
  async (mode) => {
    const fixture = setup([runningTask("managed", "managed-model-detection", "41")]);
    for (const request of [
      { targets: [{ account_id: "42", model: "test-model" }], timeout_seconds: 30 },
      {
        targets: [{ account_id: "43", model: "test-model" }],
        timeout_seconds: 30,
        mode: "precheck" as const,
      },
      {
        targets: [],
        timeout_seconds: 30,
        custom: {
          base_url: "https://example.test",
          api_key: "test",
          platform: "openai" as const,
          model: "test-model",
        },
      },
    ]) {
      await act(async () => {
        expect(await fixture.view.result.current.start(request)).toBe(true);
      });
    }
    await act(async () => {
      await fixture.client.refetchQueries({
        queryKey: ["model-animation", "history"],
        exact: true,
      });
    });
    await act(async () => fixture.view.result.current.cancelActive(mode));
    expect(fixture.deleted).toEqual([mode === "animation" ? "local-42" : "local-43"]);
    expect(fixture.view.result.current.busyIDs.has("41")).toBe(true);
    expect(fixture.view.result.current.busyIDs.has(mode === "animation" ? "43" : "42")).toBe(true);
    expect(fixture.view.result.current.busyIDs.has("custom")).toBe(true);
  },
);

it("本页任务已经结束时不再显示取消入口且不发送取消请求", async () => {
  const fixture = setup([]);
  await act(async () => {
    await fixture.view.result.current.start({
      targets: [{ account_id: "42", model: "test-model" }],
      timeout_seconds: 30,
    });
  });
  await act(async () => {
    fixture.client.setQueryData<Task>(
      ["model-animation", "task", "local-42"],
      (task) => task && { ...task, status: "succeeded" },
    );
  });
  await act(async () => fixture.view.result.current.cancelActive());
  expect(fixture.deleted).toEqual([]);
  await waitFor(() => expect(fixture.view.result.current.cancellableTaskIDs.animation).toEqual([]));
});

it("离开页面后重新进入，已有任务继续展示但不会成为本页可批量取消的任务", async () => {
  const fixture = setup([]);
  await act(async () => {
    await fixture.view.result.current.start({
      targets: [{ account_id: "42", model: "test-model" }],
      timeout_seconds: 30,
    });
  });
  fixture.view.unmount();
  const view = renderHook(() => useAnimationTasks(), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={fixture.client}>{props.children}</QueryClientProvider>
    ),
  });
  await act(async () => view.result.current.cancelActive());
  expect(fixture.deleted).toEqual([]);
  expect(view.result.current.busyIDs.has("42")).toBe(true);
});
