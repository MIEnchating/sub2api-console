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

it.each([
  ["running", true, false, "前置检测 · 等待检测结果"],
  ["running", false, true, "终端检测 · 等待检测结果"],
  ["cancelled", true, false, "前置检测 · 任务已取消，未取得结果"],
  ["failed", false, true, "终端检测 · 任务失败，未取得结果"],
] as const)(
  "%s 任务关闭动画且选择前置 %s / 终端 %s 时不显示未选择阶段的占位",
  async (status, precheck, terminal, label) => {
    setup({
      ...task,
      status,
      result: {
        ...task.result,
        animations: [],
        account_ids: ["41"],
        account_names_by_id: { "41": "待检账号" },
        configuration: { group_ids: ["7"], animation: false, precheck, terminal },
      },
    });
    const card = await screen.findByRole("article", { name: "检测账号 待检账号" });
    expect(within(card).getByText(label)).toBeVisible();
    expect(card).not.toHaveTextContent("动画检测");
    expect(card).not.toHaveTextContent(precheck ? "终端检测" : "前置检测");
  },
);
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
  expect(screen.getByRole("tab", { name: /主分组/ })).toHaveTextContent("1");
  expect(within(main).queryByRole("heading", { name: "主分组" })).not.toBeInTheDocument();
  expect(within(main).queryByText("1 个账号")).not.toBeInTheDocument();
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
    await client.invalidateQueries({ queryKey: ["task", "run-1"] });
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
  expect(within(card).getByText("动画检测 · 任务已取消，未取得结果")).toBeVisible();
  expect(within(card).queryByText(/等待检测结果/)).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: /备用分组/ }));
  expect(await screen.findByRole("region", { name: "分组 备用分组" })).toHaveTextContent(
    "暂无账号结果",
  );
});

it("任务超时后未保存结果的阶段展示超时原因，已有失败结果保留原始详情", async () => {
  setup({
    ...task,
    status: "failed",
    message: "任务执行失败：context deadline exceeded",
    result: {
      ...task.result,
      error: "context deadline exceeded",
      configuration: { group_ids: ["7"], precheck: true, terminal: true, animation: true },
      group_ids_by_account: { "41": ["7"], "42": ["7"] },
      account_names_by_id: { "41": "未完成账号", "42": "失败账号" },
      animations: [
        { ...(task.result.animations as Record<string, unknown>[])[1], account_name: "失败账号" },
      ],
    },
  });
  const card = await screen.findByRole("article", { name: "检测账号 未完成账号" });
  for (const stage of ["前置检测", "终端检测", "动画检测"]) {
    expect(
      within(card).getByRole("group", { name: `${stage} · 任务超时，未取得结果` }),
    ).toHaveClass("text-warning");
  }
  expect(
    within(card).getAllByText("任务已超时结束；没有结果的阶段可能尚未执行或执行中断，请重新检测。"),
  ).toHaveLength(1);
  expect(card).not.toHaveTextContent("请求失败");
  expect(screen.queryByText(/context deadline exceeded/)).not.toBeInTheDocument();
  const failed = screen.getByRole("article", { name: "检测账号 失败账号" });
  expect(within(failed).getByRole("group", { name: "动画检测 · 失败" })).toBeVisible();
  const user = userEvent.setup();
  await user.click(within(failed).getByRole("button", { name: "查看动画检测详情" }));
  expect(await screen.findByRole("dialog", { name: "动画检测详情" })).toHaveTextContent("上游失败");
});

it.each([
  [
    "failed",
    "账号绑定已变化，请重新提交",
    "任务失败，未取得结果",
    "任务失败：账号绑定已变化，请重新提交。没有结果的阶段无法确认是否执行，请重新检测。",
  ],
  [
    "cancelled",
    "检测任务已取消",
    "任务已取消，未取得结果",
    "任务已取消；没有结果的阶段可能尚未执行或执行中断，需要时可重新检测。",
  ],
  [
    "succeeded",
    "检测任务完成",
    "结果未记录",
    "任务已结束，但未保存该阶段结果，无法确认是否执行；请重新检测。",
  ],
] as const)(
  "%s 任务缺失结果时展示准确原因而不冒充账号请求错误",
  async (status, message, label, reason) => {
    setup({
      ...task,
      status,
      message,
      result: {
        ...task.result,
        animations: [],
        account_ids: ["41"],
        configuration: { group_ids: ["7"], precheck: true, animation: false },
      },
    });
    const card = await screen.findByRole("article", { name: "检测账号 账号 41" });
    expect(within(card).getByRole("group", { name: `前置检测 · ${label}` })).toBeVisible();
    expect(within(card).getByText(reason)).toBeVisible();
    expect(
      within(card).queryByRole("button", { name: "查看前置检测详情" }),
    ).not.toBeInTheDocument();
  },
);

it.each([
  ["account-model-animation", ["动画检测"]],
  ["account-model-precheck", ["前置检测"]],
  ["account-model-combined", ["前置检测", "动画检测"]],
  ["account-terminal-continuity", ["终端检测"]],
] as const)(
  "从系统信息打开 %s 时仅展示本次执行阶段，不显示分组缺失提示",
  async (operation, stages) => {
    setup({
      ...task,
      operation,
      status: "running",
      progress: 0,
      result: { account_ids: ["41"], animations: [], checks: [] },
    });
    const card = await screen.findByRole("article", { name: "检测账号 账号 41" });
    for (const stage of ["前置检测", "动画检测", "终端检测"]) {
      const status = within(card).queryByRole("group", { name: new RegExp(`^${stage} ·`) });
      if ((stages as readonly string[]).includes(stage)) expect(status).toBeVisible();
      else expect(status).not.toBeInTheDocument();
    }
    expect(screen.queryByText(/旧记录未保存执行时的分组归属/)).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "检测账号 1" })).toBeVisible();
  },
);
