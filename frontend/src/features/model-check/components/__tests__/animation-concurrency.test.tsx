import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationRequest, Task } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

const history: Task = {
  id: "history",
  skill: "sub2api-model-animation",
  operation: "account-model-animation",
  status: "failed",
  progress: 100,
  message: "失败",
  created_at: "2026-09-13T00:00:00Z",
  updated_at: "2026-09-13T00:00:00Z",
  result: {
    animations: ["41", "42"].map((id) => ({
      account_id: id,
      account_name: id,
      model: "saved-model",
      request_id: id,
      status: "failed",
      error: "上游超时",
      duration_ms: 1000,
      completed_at: "2026-09-13T00:00:00Z",
    })),
  },
};
function running(id: string): Task {
  return {
    ...history,
    id: `retry-${id}`,
    status: "running",
    progress: 0,
    created_at: "2026-09-13T01:00:00Z",
    result: { account_ids: [id], animations: [] },
  };
}
const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
afterEach(() => {
  cleanup();
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.unstubAllGlobals();
});
function setup(active: Task[] = []) {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = createConsoleQueryClient();
  clients.push(client);
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    ["41", "42", "43"].map((id) => ({ id, name: id, groups: [], platform: "openai" })),
  );
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], [...active, history]);
  for (const task of [...active, history])
    client.setQueryData(["model-animation", "task", task.id], task);
  const posts: { request: AnimationRequest; resolve: (response: Response) => void }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST")
        return new Promise<Response>((resolve) =>
          posts.push({ request: JSON.parse(String(init.body)) as AnimationRequest, resolve }),
        );
      const id = String(input).split("/").at(-1);
      if (String(input).includes("/api/tasks/"))
        return Promise.resolve(Response.json(client.getQueryData(["model-animation", "task", id])));
      return Promise.resolve(Response.json([...active, history]));
    }),
  );
  render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return { client, posts };
}
function card(id: string) {
  return within(screen.getByRole("article", { name: `账号 ${id}` }));
}

it("重试请求尚未返回时在本账号显示启动中，其他账号可继续重试并开始检测", async () => {
  const view = setup();
  fireEvent.click(card("41").getByRole("button", { name: "重试 41" }));
  expect(await card("41").findByRole("status", { name: "正在启动检测" })).toBeVisible();
  expect(card("41").getByRole("checkbox")).toHaveAttribute("aria-disabled", "true");
  expect(card("42").getByRole("button", { name: "重试 42" })).toBeEnabled();
  fireEvent.click(card("42").getByRole("button", { name: "重试 42" }));
  expect(await card("42").findByRole("status", { name: "正在启动检测" })).toBeVisible();
  await act(async () => {
    view.posts[0]!.resolve(Response.json(running("41")));
  });
  expect(await card("41").findByRole("status", { name: "生成中，等待动画结果" })).toBeVisible();
  expect(card("42").getByRole("status", { name: "正在启动检测" })).toBeVisible();
  fireEvent.click(card("43").getByRole("checkbox"));
  fireEvent.change(screen.getByRole("combobox", { name: "检测模型" }), {
    target: { value: "new-model" },
  });
  fireEvent.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  const dialog = await screen.findByRole("dialog", { name: "确认动画检测范围" });
  fireEvent.click(within(dialog).getByRole("button", { name: "确认并开始检测" }));
  await waitFor(() => expect(view.posts).toHaveLength(3));
  expect(view.posts[2]!.request.targets).toEqual([{ account_id: "43", model: "new-model" }]);
  await act(async () => {
    view.posts[1]!.resolve(Response.json(running("42")));
    view.posts[2]!.resolve(Response.json(running("43")));
  });
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  for (const id of ["41", "42", "43"])
    expect(card(id).getByRole("status", { name: "生成中，等待动画结果" })).toBeVisible();
});

it("恢复多个进行中任务时各卡片独立显示状态，一个完成后不影响其他任务", async () => {
  const view = setup([running("41"), running("42")]);
  expect(await card("41").findByRole("status", { name: "生成中，等待动画结果" })).toBeVisible();
  expect(card("42").getByRole("status", { name: "生成中，等待动画结果" })).toBeVisible();
  await act(async () =>
    view.client.setQueryData(["model-animation", "task", "retry-41"], {
      ...running("41"),
      status: "cancelled",
    }),
  );
  expect(await card("41").findByRole("button", { name: "重试 41" })).toBeEnabled();
  expect(card("42").getByRole("status", { name: "生成中，等待动画结果" })).toBeVisible();
});

it("一个账号启动失败时恢复旧结果和重试入口，另一个账号仍显示启动中", async () => {
  const view = setup();
  fireEvent.click(card("41").getByRole("button", { name: "重试 41" }));
  fireEvent.click(card("42").getByRole("button", { name: "重试 42" }));
  await waitFor(() => expect(view.posts).toHaveLength(2));
  await act(async () =>
    view.posts[0]!.resolve(Response.json({ detail: "启动失败" }, { status: 503 })),
  );
  expect(await card("41").findByRole("button", { name: "重试 41" })).toBeEnabled();
  expect(card("41").getByText("上游超时")).toBeVisible();
  expect(card("42").getByRole("status", { name: "正在启动检测" })).toBeVisible();
});
