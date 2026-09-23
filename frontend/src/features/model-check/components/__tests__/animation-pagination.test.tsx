import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(
  count: number,
  unavailable = false,
): { dispose: () => void; posts: { targets: { account_id: string; model: string }[] }[] } {
  const posts: { targets: { account_id: string; model: string }[] }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") posts.push(JSON.parse(String(init.body)));
      if (init?.method !== "POST" && !String(_input).includes("/api/tasks/"))
        return Response.json([]);
      return Response.json({ id: "pagination-task", status: "succeeded", result: {} });
    }),
  );
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    Array.from({ length: count }, (_, index) => ({
      id: String(index + 1),
      name: `分页账号 ${index + 1}`,
      groups: [],
      platform: unavailable && index === 0 ? "gemini" : "openai",
    })),
  );
  client.setQueryData(["model-animation", "schedules"], []);
  const history: Task[] = unavailable
    ? [
        {
          id: "busy-animation",
          skill: "sub2api-model-animation",
          operation: "account-model-animation",
          status: "running",
          progress: 1,
          message: "正在检测",
          result: { account_ids: ["2"], animations: [] },
          created_at: "2026-09-22T00:00:00Z",
          updated_at: "2026-09-22T00:00:00Z",
        },
      ]
    : [];
  client.setQueryData(["model-animation", "history"], history);
  for (const task of history) client.setQueryData(["model-animation", "task", task.id], task);
  const view = render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return {
    posts,
    dispose: () => {
      view.unmount();
      client.clear();
    },
  };
}

it("账号较多时默认只挂载 12 张卡片，通过分页访问剩余账号", async () => {
  const view = setup(60);
  expect(screen.getAllByRole("checkbox", { name: /检测 分页账号/ })).toHaveLength(12);
  expect(screen.queryByRole("checkbox", { name: /^检测 分页账号 13\b/ })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(await screen.findByRole("checkbox", { name: /^检测 分页账号 13\b/ })).toBeVisible();
  expect(screen.getAllByRole("checkbox", { name: /检测 分页账号/ })).toHaveLength(12);
  view.dispose();
});

it("跨页勾选和搜索后保留统一模型，并提交完整的已选范围", async () => {
  const user = userEvent.setup();
  const view = setup(25);
  await user.click(screen.getByRole("checkbox", { name: /^检测 分页账号 1\b/ }));
  await user.type(screen.getByRole("combobox", { name: "检测模型" }), "first-model");
  await user.keyboard("{Escape}");
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  await user.click(screen.getByRole("checkbox", { name: /^检测 分页账号 13\b/ }));
  const search = screen.getByRole("textbox", { name: "搜索动画检测账号" });
  await user.type(search, "分页账号 25");
  expect(await screen.findByRole("checkbox", { name: /^检测 分页账号 25\b/ })).toBeVisible();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  await user.clear(search);
  expect(await screen.findByRole("checkbox", { name: /^检测 分页账号 1\b/ })).toBeChecked();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveValue("first-model");
  await user.click(screen.getByRole("button", { name: "开始检测（2 个账号）" }));
  await waitFor(() =>
    expect(view.posts[0]?.targets).toEqual([
      { account_id: "1", model: "first-model" },
      { account_id: "13", model: "first-model" },
    ]),
  );
  view.dispose();
});

it("全选跨页选择全部账号，取消后可重新勾选第二十一个账号并确认完整范围", async () => {
  const view = setup(25);
  fireEvent.click(screen.getByRole("button", { name: "全选账号" }));
  expect(screen.getByRole("button", { name: "开始检测（25 个账号）" })).toBeEnabled();
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  const account = screen.getByRole("checkbox", { name: /^检测 分页账号 21\b/ });
  expect(account).toBeChecked();
  fireEvent.click(account);
  expect(account).not.toBeChecked();
  expect(account).not.toHaveAttribute("aria-disabled", "true");
  fireEvent.click(account);
  expect(account).toBeChecked();
  fireEvent.change(screen.getByRole("combobox", { name: "检测模型" }), {
    target: { value: "shared-model" },
  });
  fireEvent.click(screen.getByRole("button", { name: "开始检测（25 个账号）" }));
  await waitFor(() => expect(view.posts[0]?.targets).toHaveLength(25));
  expect(view.posts[0]?.targets).toContainEqual({ account_id: "25", model: "shared-model" });
  fireEvent.click(screen.getByRole("button", { name: "清空选择" }));
  expect(screen.getByRole("button", { name: "开始检测（0 个账号）" })).toBeDisabled();
  view.dispose();
});

it("搜索后全选只选择当前筛选范围，空结果时禁用全选", () => {
  const view = setup(25);
  const search = screen.getByRole("textbox", { name: "搜索动画检测账号" });
  fireEvent.change(search, { target: { value: "分页账号 25" } });
  fireEvent.click(screen.getByRole("button", { name: "全选账号" }));
  expect(screen.getByRole("button", { name: "开始检测（1 个账号）" })).toBeEnabled();
  expect(screen.getByRole("checkbox", { name: /^检测 分页账号 25\b/ })).toBeChecked();
  fireEvent.change(search, { target: { value: "没有此账号" } });
  expect(screen.getByRole("button", { name: "全选账号" })).toBeDisabled();
  view.dispose();
});

it("翻页后提交缺少统一模型时，聚焦顶部模型字段", async () => {
  const view = setup(25);
  fireEvent.click(screen.getByRole("checkbox", { name: /^检测 分页账号 1\b/ }));
  fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  fireEvent.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  const model = screen.getByRole("combobox", { name: "检测模型" });
  await waitFor(() => expect(model).toHaveFocus());
  expect(model).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByText("请输入模型 ID")).toBeVisible();
  view.dispose();
});

it("全选排除正在检测和不支持平台的账号", () => {
  const view = setup(25, true);
  fireEvent.click(screen.getByRole("button", { name: "全选账号" }));
  expect(screen.getByRole("button", { name: "开始检测（23 个账号）" })).toBeEnabled();
  expect(screen.getByRole("checkbox", { name: /^检测 分页账号 1\b/ })).not.toBeChecked();
  expect(screen.getByRole("checkbox", { name: /^检测 分页账号 2\b/ })).not.toBeChecked();
  expect(screen.getByRole("checkbox", { name: /^检测 分页账号 3\b/ })).toBeChecked();
  view.dispose();
});
