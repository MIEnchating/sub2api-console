import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createConsoleQueryClient } from "@/lib/query-client";
import type { Task } from "@/api";
import { AnimationCheckPanel } from "../animation-check-panel";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(
  task: Task,
  previous?: Task,
): { dispose: () => void; client: ReturnType<typeof createConsoleQueryClient> } {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    [
      { id: "41", name: "甲账号", groups: [], platform: "openai" },
      { id: "42", name: "乙账号", groups: [], platform: "openai" },
      { id: "43", name: "丙账号", groups: [], platform: "openai" },
    ],
  );
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], previous ? [task, previous] : [task]);
  if (previous) client.setQueryData(["model-animation", "task", previous.id], previous);
  client.setQueryData(["model-animation", "task", task.id], task);
  const view = render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return {
    dispose: () => {
      view.unmount();
      client.clear();
    },
    client,
  };
}
const task: Task = {
  id: "task-1",
  skill: "sub2api-model-animation",
  operation: "account-model-animation",
  status: "partial",
  progress: 100,
  message: "成功 1，失败 1",
  created_at: "2026-09-13T00:00:00Z",
  updated_at: "2026-09-13T00:00:00Z",
  result: {
    account_ids: ["41", "42"],
    targets: [
      { account_id: "41", model: "model-a" },
      { account_id: "42", model: "model-b" },
    ],
    animations: [
      {
        account_id: "42",
        account_name: "乙账号",
        model: "model-b",
        request_id: "r2",
        status: "failed",
        error: "余额不足",
        duration_ms: 20,
        completed_at: "2026-09-13T00:00:00Z",
      },
      {
        account_id: "41",
        account_name: "甲账号",
        model: "model-a",
        request_id: "r1",
        status: "succeeded",
        svg: '<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>',
        duration_ms: 30,
        completed_at: "2026-09-13T00:00:00Z",
      },
    ],
  },
};

it("展示动画账号时卡片和检测面板沿用统一圆角", () => {
  const { dispose } = setup(task);
  for (const card of screen.getAllByRole("article")) expect(card).toHaveClass("rounded-lg");
  expect(screen.getByRole("tablist", { name: "动画检测来源" }).closest(".bg-card")).toHaveClass(
    "rounded-lg",
  );
  dispose();
});

it("检测结果顺序与账号不同，仍在对应账号卡片中展示动画或失败原因", () => {
  const { dispose } = setup(task);
  const cards = screen.getAllByRole("article");
  const first = cards.find((card) =>
    within(card).queryByRole("checkbox", { name: /检测 甲账号/ }),
  )!;
  const second = cards.find((card) =>
    within(card).queryByRole("checkbox", { name: /检测 乙账号/ }),
  )!;
  expect(within(first).getByRole("img", { name: /甲账号生成/ })).toBeVisible();
  expect(within(second).getByText("余额不足")).toBeVisible();
  expect(within(second).getByRole("button", { name: "重试 乙账号" })).toBeEnabled();
  expect(cards).toHaveLength(3);
  dispose();
});

it("已有结果自动展示且不提供历史切换，无记录账号显示待检测状态", () => {
  const { dispose } = setup(task);
  expect(screen.queryByRole("button", { name: "仅看本次检测" })).not.toBeInTheDocument();
  expect(screen.queryByRole("combobox", { name: "最近检测任务" })).not.toBeInTheDocument();
  expect(screen.queryByText(/匹配.*个账号/)).not.toBeInTheDocument();
  const emptyCard = screen.getByRole("article", { name: "账号 丙账号" });
  expect(within(emptyCard).getByText("尚未检测")).toBeVisible();
  expect(within(emptyCard).queryByRole("button", { name: /放大查看/ })).not.toBeInTheDocument();
  expect(screen.getAllByRole("article")).toHaveLength(3);
  dispose();
});

it("搜索和操作分行，开始检测位于顶部，分页独立于卡片滚动区域", () => {
  const { dispose } = setup(task);
  const filters = screen.getByRole("group", { name: "动画账号筛选" });
  const operations = screen.getByRole("group", { name: "动画检测操作" });
  expect(
    filters.compareDocumentPosition(operations) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(within(operations).getByRole("button", { name: /开始检测/ })).toBeVisible();
  const region = screen.getByRole("region", { name: "动画账号卡片" });
  expect(region).toHaveClass("overflow-visible", "md:overflow-y-auto");
  expect(within(region).queryByRole("button", { name: "转到下一页" })).not.toBeInTheDocument();
  for (const card of screen.getAllByRole("article"))
    expect(card).toHaveClass("h-auto", "overflow-hidden");
  dispose();
});

it.each([
  ["running", "生成中，等待动画结果"],
  ["cancelled", "检测已取消，未返回动画"],
  ["failed", "本次检测未返回动画"],
] as const)("任务为 %s 且账号未返回结果时，仅任务内卡片展示对应状态", (status, label) => {
  const { dispose } = setup({ ...task, status, result: { ...task.result, animations: [] } });
  const first = screen.getByRole("article", { name: "账号 甲账号" });
  const unrelated = screen.getByRole("article", { name: "账号 丙账号" });
  expect(within(first).getByText(label)).toBeVisible();
  expect(within(unrelated).queryByText("暂无动画结果")).not.toBeInTheDocument();
  expect(within(unrelated).queryByText(label)).not.toBeInTheDocument();
  dispose();
});

it("较旧任务补充其他账号结果，同一账号保留最新结果", () => {
  const previous: Task = {
    ...task,
    id: "previous",
    created_at: "2026-09-12T00:00:00Z",
    result: {
      account_ids: ["43", "42"],
      animations: [
        {
          account_id: "42",
          account_name: "乙账号",
          model: "outdated-model",
          request_id: "outdated-result",
          status: "succeeded",
          svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
          duration_ms: 20,
          completed_at: "2026-09-12T00:00:00Z",
        },

        {
          account_id: "43",
          account_name: "丙账号",
          model: "previous-model",
          request_id: "previous-result",
          status: "succeeded",
          svg: '<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>',
          duration_ms: 50,
          completed_at: "2026-09-12T00:00:00Z",
        },
      ],
    },
  };
  const { dispose } = setup(task, previous);
  expect(screen.queryByText("outdated-model")).not.toBeInTheDocument();
  expect(screen.getByText("previous-model")).toBeVisible();
  expect(screen.getByRole("img", { name: /甲账号生成/ })).toBeVisible();
  expect(
    within(screen.getByRole("article", { name: "账号 丙账号" })).getByRole("img"),
  ).toBeVisible();
  expect(
    within(screen.getByRole("article", { name: "账号 乙账号" })).getByText("余额不足"),
  ).toBeVisible();
  dispose();
});

it.each([0, 50])("任务进度为 %s 时仅卡片显示检测状态，取消入口留在操作行", (progress) => {
  const { dispose } = setup({
    ...task,
    status: "running",
    progress,
    message: "正在生成鹈鹕骑自行车动画",
    result: { ...task.result, animations: [] },
  });
  const settings = screen.getByRole("group", { name: "动画检测设置" });
  const operations = within(settings).getByRole("group", { name: "动画检测操作" });
  expect(within(settings).queryByText("正在生成鹈鹕骑自行车动画")).not.toBeInTheDocument();
  expect(within(settings).queryByRole("progressbar")).not.toBeInTheDocument();
  expect(within(operations).getByRole("button", { name: "取消任务" })).toBeEnabled();
  expect(
    within(screen.getByRole("article", { name: "账号 甲账号" })).getByText("生成中，等待动画结果"),
  ).toBeVisible();
  dispose();
});

it("在操作行取消动画任务后恢复开始检测入口", async () => {
  const cancelled = {
    ...task,
    status: "cancelled" as const,
    result: { ...task.result, animations: [] },
  };
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "DELETE") return Response.json({ cancelled: true });
    if (String(input).endsWith("/model-checks/animations")) return Response.json([cancelled]);
    return Response.json(cancelled);
  });
  vi.stubGlobal("fetch", fetch);
  const { dispose } = setup({
    ...task,
    status: "running",
    progress: 0,
    result: { ...task.result, animations: [] },
  });
  const operations = screen.getByRole("group", { name: "动画检测操作" });
  fireEvent.click(within(operations).getByRole("button", { name: "取消任务" }));
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/api/tasks/task-1"),
      expect.objectContaining({ method: "DELETE", credentials: "include" }),
    ),
  );
  expect(await within(operations).findByRole("button", { name: /开始检测/ })).toBeDisabled();
  expect(
    within(screen.getByRole("article", { name: "账号 甲账号" })).getByText(
      "检测已取消，未返回动画",
    ),
  ).toBeVisible();
  dispose();
});

it("检测记录读取失败时在操作行提供重试，恢复后保留卡片结果", async () => {
  let failed = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      if (failed) throw new Error("检测记录读取失败");
      return Response.json([task]);
    }),
  );
  const { dispose, client } = setup(task);
  const operations = screen.getByRole("group", { name: "动画检测操作" });
  const preview = screen.getByRole("img", { name: /甲账号生成/ });
  await client.invalidateQueries({
    queryKey: ["model-animation", "history"],
    refetchType: "active",
  });
  const retry = await within(operations).findByRole("button", { name: "重新读取检测记录" });
  failed = false;
  fireEvent.click(retry);
  await waitFor(() =>
    expect(
      within(operations).queryByRole("button", { name: "重新读取检测记录" }),
    ).not.toBeInTheDocument(),
  );
  expect(screen.getByRole("img", { name: /甲账号生成/ })).toBe(preview);
  dispose();
});
