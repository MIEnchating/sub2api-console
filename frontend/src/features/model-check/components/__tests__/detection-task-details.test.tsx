import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { DetectionTaskDetails } from "../detection-task-details";
import { DetectionResultCard } from "../detection-result-card";

const task: Task = {
  id: "run-1",
  skill: "sub2api-model-animation",
  operation: "managed-model-detection",
  status: "succeeded",
  progress: 100,
  message: "检测任务完成",
  created_at: "2026-09-23T00:00:00Z",
  updated_at: "2026-09-23T00:01:00Z",
  result: {
    account_ids: ["41", "42"],
    configuration: { group_ids: ["7", "8"] },
    group_ids_by_account: { "41": ["7"], "42": ["8"] },
    animations: [
      {
        account_id: "41",
        account_name: "主组账号",
        model: "test-model",
        status: "succeeded",
        svg: '<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>',
        request_id: "animation-41",
        duration_ms: 10,
        completed_at: "2026-09-23T00:01:00Z",
      },
      {
        account_id: "42",
        account_name: "备用组账号",
        model: "test-model",
        status: "failed",
        error: "上游失败",
        request_id: "animation-42",
        duration_ms: 10,
        completed_at: "2026-09-23T00:01:00Z",
      },
    ],
  },
};

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function setup(value: Task = task): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(
    ["groups"],
    [
      { id: "7", name: "主分组" },
      { id: "8", name: "备用分组" },
    ],
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(value)),
  );
  render(
    <QueryClientProvider client={client}>
      <DetectionTaskDetails id="run-1" onClose={vi.fn()} />
    </QueryClientProvider>,
  );
  return client;
}
it("账号名称过长时可通过键盘聚焦标题查看完整名称并按 Escape 关闭提示", async () => {
  const name = "检测账号的完整长名称".repeat(12);
  render(
    <DetectionResultCard
      row={{ id: "41", name }}
      active={false}
      precheck={false}
      terminal={false}
    />,
  );
  const heading = screen.getByRole("heading", { name });
  const user = userEvent.setup();
  await user.tab();
  expect(heading).toHaveFocus();
  expect(await screen.findByRole("tooltip")).toHaveTextContent(name);
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  expect(heading).toHaveFocus();
});

it("运行详情按任务选择的分组展示账号且每个账号只出现在所属分组", async () => {
  setup();
  const main = await screen.findByRole("region", { name: "分组 主分组" });
  expect(within(main).getByRole("article")).toHaveTextContent("主组账号");
  expect(screen.getByRole("tab", { name: /主分组/ })).toHaveAttribute("aria-selected", "true");
  expect(screen.queryByRole("article", { name: "检测账号 备用组账号" })).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: /备用分组/ }));
  const backup = await screen.findByRole("region", { name: "分组 备用分组" });
  expect(within(backup).getByRole("article")).toHaveTextContent("备用组账号");
  expect(screen.queryByRole("article", { name: "检测账号 主组账号" })).not.toBeInTheDocument();
  expect(screen.getByRole("tab", { name: /备用分组/ })).toHaveAttribute("aria-selected", "true");
});

it("使用方向键切换分组时更新选中状态并关联对应结果面板", async () => {
  setup();
  const user = userEvent.setup();
  const main = await screen.findByRole("tab", { name: /主分组/ });
  main.focus();
  await user.keyboard("{ArrowRight}");
  const backup = screen.getByRole("tab", { name: /备用分组/ });
  expect(backup).toHaveFocus();
  await user.keyboard("{Enter}");
  expect(backup).toHaveAttribute("aria-selected", "true");
  const panel = screen.getByRole("tabpanel", { name: /备用分组/ });
  expect(backup).toHaveAttribute("aria-controls", panel.id);
  expect(within(panel).getByRole("article")).toHaveTextContent("备用组账号");
});

it("后台刷新检测结果后保留用户当前选择的分组", async () => {
  const client = setup();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("tab", { name: /备用分组/ }));
  vi.mocked(fetch).mockResolvedValueOnce(Response.json({ ...task, message: "检测结果已更新" }));
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["model-detection-tasks", "run", "run-1"] });
  });
  expect(await screen.findByText("检测结果已更新")).toBeVisible();
  expect(screen.getByRole("tab", { name: /备用分组/ })).toHaveAttribute("aria-selected", "true");
  expect(screen.getByRole("article")).toHaveTextContent("备用组账号");
});

it("账号同时属于多个选中分组时只在选择顺序首组展示，保存名称优先于当前名称", async () => {
  setup({
    ...task,
    result: {
      ...task.result,
      configuration: { group_ids: ["8", "7"] },
      group_names_by_id: { "7": "原主分组", "8": "原备用分组" },
      group_ids_by_account: { "41": ["7", "8"], "42": ["8"] },
    },
  });
  const backup = await screen.findByRole("region", { name: "分组 原备用分组" });
  expect(within(backup).getAllByRole("article")).toHaveLength(2);
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: /原主分组/ }));
  const main = await screen.findByRole("region", { name: "分组 原主分组" });
  expect(within(main).queryAllByRole("article")).toHaveLength(0);
  expect(screen.getByRole("tab", { name: /原备用分组/ })).toHaveAttribute("aria-selected", "false");
});
it("长错误与成功动画都使用固定预览高度，键盘打开完整错误并返回原入口", async () => {
  const animations = task.result.animations as Record<string, unknown>[];
  const error = "上游直连接口返回 HTTP 502：" + "长错误信息".repeat(80);
  setup({
    ...task,
    result: { ...task.result, animations: [animations[0], { ...animations[1], error }] },
  });
  const success = await screen.findByRole("article", { name: "检测账号 主组账号" });
  expect(within(success).getByRole("group", { name: "动画预览区域" })).toHaveClass(
    "h-[180px]",
    "overflow-hidden",
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: /备用分组/ }));
  const failure = await screen.findByRole("article", { name: "检测账号 备用组账号" });
  expect(within(failure).getByRole("group", { name: "动画预览区域" })).toHaveClass(
    "h-[180px]",
    "overflow-hidden",
  );
  expect(within(failure).getByText(error)).toHaveClass("line-clamp-2", "wrap-anywhere");
  const trigger = within(failure).getByRole("button", { name: "查看动画检测详情" });
  trigger.focus();
  await user.keyboard("{Enter}");
  const detail = screen.getByRole("dialog", { name: "动画检测详情" });
  expect(within(detail).getByText(error)).toBeVisible();
  await user.keyboard("{Escape}");
  expect(trigger).toHaveFocus();
});
it("旧多分组记录缺少归属时保留结果但不猜测历史分组", async () => {
  const result = { ...task.result };
  delete result.group_ids_by_account;
  setup({ ...task, result });
  const user = userEvent.setup();
  await screen.findByRole("region", { name: "分组 主分组" });
  await user.click(screen.getByRole("tab", { name: /分组未记录/ }));
  expect(await screen.findByRole("region", { name: "分组 分组未记录" })).toHaveTextContent(
    "主组账号",
  );
  expect(screen.getByText("旧记录未保存执行时的分组归属，保留原结果供查看。")).toBeVisible();
});
it("已取消且未返回结果的账号不继续显示等待，空分组保留入口", async () => {
  setup({
    ...task,
    status: "cancelled",
    result: {
      ...task.result,
      account_ids: ["41"],
      animations: [],
      group_ids_by_account: { "41": ["7"] },
      account_names_by_id: { "41": "待检账号" },
      configuration: { group_ids: ["7", "8"], precheck: true, terminal: true },
    },
  });
  const card = await screen.findByRole("article", { name: "检测账号 待检账号" });
  expect(within(card).getByText("动画检测 · 未返回结果")).toBeVisible();
  expect(within(card).queryByText(/等待检测结果/)).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: /备用分组/ }));
  expect(await screen.findByRole("region", { name: "分组 备用分组" })).toHaveTextContent(
    "暂无账号结果",
  );
});
