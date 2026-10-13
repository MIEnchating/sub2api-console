import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

const endpoint = `https://custom.example.invalid/${"gateway/".repeat(30)}v1`;
const source = {
  endpoint,
  platform: "anthropic",
  model: "requested-model",
  account_id: "custom-record",
};
const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
afterEach(() => {
  for (const client of clients.splice(0)) client.clear();
  vi.restoreAllMocks();
});

async function setup(
  mode: "animation" | "precheck",
  status: Task["status"],
  legacy = false,
  records?: Task[],
): Promise<void> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
  const result = {
    ...(legacy ? { account_id: source.account_id, model: source.model } : source),
    account_name: "自定义接口",
    mode,
    response_model: "returned-model",
    status,
    svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
    request_id: "request-custom",
    duration_ms: 1234,
    completed_at: "2026-09-21T00:00:00Z",
  };
  const task: Task = {
    id: "custom-task",
    skill: "sub2api-model-animation",
    operation: `account-model-${mode}`,
    status,
    message: "检测",
    progress: 0,
    created_at: result.completed_at,
    updated_at: result.completed_at,
    result: {
      targets: [legacy ? { account_id: source.account_id, model: source.model } : source],
      account_ids: [source.account_id],
      animations: status === "succeeded" || status === "failed" ? [result] : [],
    },
  };
  const client = createConsoleQueryClient();
  seedDictionaries(client);
  clients.push(client);
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(["accounts"], []);
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "quality-history"], []);
  client.setQueryData(["model-animation", "history"], records ?? [task]);
  for (const record of records ?? [task]) {
    client.setQueryData(["model-animation", "task", record.id], record);
  }
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(task)),
  );
  render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByRole("tab", { name: "自定义接口" }));
  if (mode === "precheck") {
    await userEvent.click(
      within(screen.getByRole("tablist", { name: "自定义检测记录类型" })).getByRole("tab", {
        name: "前置检测",
      }),
    );
  }
}

it.each(["animation", "precheck"] as const)(
  "%s 历史记录展示原接口和模型，键盘可打开完整详情",
  async (mode) => {
    await setup(mode, "succeeded");
    const card = screen.getByRole("article", { name: "自定义检测 requested-model" });
    const header = within(card).getByRole("group", { name: "检测接口信息" });
    expect(within(header).getByText(endpoint)).toBeVisible();
    expect(header).toHaveTextContent("Anthropic");
    expect(within(header).queryByText(/^(动画检测|前置检测)$/)).not.toBeInTheDocument();
    expect(card).not.toHaveTextContent("request-custom");
    expect(within(card).getByText(endpoint)).toHaveClass("wrap-anywhere");
    expect(card).toHaveTextContent("Anthropic");
    expect(card).toHaveTextContent("requested-model");
    const title = mode === "animation" ? "动画检测详情" : "前置检测详情";
    const trigger = within(card).getByRole("button", { name: `查看${title}` });
    trigger.focus();
    await userEvent.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", { name: title });
    expect(dialog).toHaveTextContent(endpoint);
    expect(dialog).toHaveTextContent("Anthropic");
    expect(dialog).toHaveTextContent("requested-model");
    expect(dialog).toHaveTextContent("returned-model");
    expect(dialog).toHaveTextContent("request-custom");
    await userEvent.keyboard("{Escape}");
    expect(trigger).toHaveFocus();
  },
);

it.each(["running", "cancelled", "failed"] as const)(
  "自定义任务为 %s 时仍能辨认原接口和请求模型",
  async (status) => {
    await setup("animation", status);
    const card = screen.getByRole("article", { name: "自定义检测 requested-model" });
    expect(card).toHaveTextContent(endpoint);
    expect(card).toHaveTextContent("Anthropic");
    expect(card).toHaveTextContent("requested-model");
  },
);

it("旧记录未保存接口地址时明确显示未记录", async () => {
  await setup("animation", "succeeded", true);
  expect(screen.getByRole("article")).toHaveTextContent("接口地址未记录");
});

it.each([
  ["precheck", "succeeded"],
  ["animation", "running"],
] as const)("%s 为 %s 时结果卡片按内容排列，不拉满剩余高度", async (mode, status) => {
  await setup(mode, status);
  const card = screen.getByRole("article");
  expect(card).toHaveClass("h-auto");
  expect(card.parentElement).toHaveClass("items-start", "shrink-0");
  expect(card.parentElement).not.toHaveClass("flex-1", "grid-rows-1");
  if (status === "running") {
    expect(within(card).getByRole("button", { name: "取消任务" })).toHaveClass("shrink-0");
    expect(within(card).queryByRole("progressbar")).not.toBeInTheDocument();
  }
});

it("成功动画使用有界预览，短窗口中结果操作不会被撑到页面底部", async () => {
  await setup("animation", "succeeded");
  const preview = screen.getByRole("button", { name: /放大查看/ });
  expect(preview.parentElement).toHaveClass("h-56", "shrink-0");
  expect(screen.getByLabelText("动画检测结果")).not.toHaveClass("h-full");
});

function historyRecord(
  id: string,
  mode: "animation" | "precheck",
  day: number,
  running = false,
): Task {
  const timestamp = `2026-10-${day}T00:00:00Z`;
  const target = { ...source, account_id: `custom-${id}`, model: id };
  return {
    id,
    skill: "sub2api-model-animation",
    operation: `account-model-${mode}`,
    status: running ? "running" : "succeeded",
    message: "检测",
    progress: running ? 0 : 100,
    created_at: timestamp,
    updated_at: timestamp,
    result: {
      account_ids: [target.account_id],
      targets: [target],
      animations: running
        ? []
        : [
            {
              ...target,
              account_name: "自定义接口",
              mode,
              status: "succeeded",
              svg: '<svg xmlns="http://www.w3.org/2000/svg"/>',
              request_id: id,
              duration_ms: 1000,
              completed_at: timestamp,
              manual_verdict: { verdict: "passed", updated_at: timestamp, updated_by: "admin" },
            },
          ],
    },
  };
}

it("自定义动画已有标记时也不展示手动标记及清除操作", async () => {
  await setup("animation", "succeeded", false, [historyRecord("marked", "animation", 12)]);
  const card = screen.getByRole("article");
  expect(within(card).queryByText("手动标记")).not.toBeInTheDocument();
  expect(within(card).queryByRole("button", { name: /降智|清除标记/ })).not.toBeInTheDocument();
});

it("两类检测分别展示且各自按启动时间倒序，尚无结果的新任务排在首位", async () => {
  await setup("animation", "succeeded", false, [
    historyRecord("animation-old", "animation", 10),
    historyRecord("precheck-new", "precheck", 12),
    historyRecord("animation-running", "animation", 13, true),
    historyRecord("precheck-old", "precheck", 10),
    historyRecord("animation-new", "animation", 12),
  ]);
  const animations = screen.getByRole("region", { name: "动画检测记录" });
  expect(screen.queryByRole("region", { name: "前置检测记录" })).not.toBeInTheDocument();
  expect(
    within(animations)
      .getAllByRole("article")
      .map((card) => card.getAttribute("aria-label")),
  ).toEqual([
    "自定义检测 animation-running",
    "自定义检测 animation-new",
    "自定义检测 animation-old",
  ]);
  const tabs = screen.getByRole("tablist", { name: "自定义检测记录类型" });
  const animationTab = within(tabs).getByRole("tab", { name: "动画检测" });
  expect(animationTab).toHaveAttribute("aria-selected", "true");
  animationTab.focus();
  await userEvent.keyboard("{ArrowRight}");
  const precheckTab = within(tabs).getByRole("tab", { name: "前置检测" });
  expect(precheckTab).toHaveFocus();
  await userEvent.keyboard("{Enter}");
  expect(precheckTab).toHaveAttribute("aria-selected", "true");
  expect(screen.queryByRole("region", { name: "动画检测记录" })).not.toBeInTheDocument();
  expect(
    within(screen.getByRole("region", { name: "前置检测记录" }))
      .getAllByRole("article")
      .map((card) => card.getAttribute("aria-label")),
  ).toEqual(["自定义检测 precheck-new", "自定义检测 precheck-old"]);
  expect(screen.queryByRole("heading", { name: "检测记录" })).not.toBeInTheDocument();
  expect(screen.queryByText(/共 \d+ 条/)).not.toBeInTheDocument();
});

it("切换检测 Tab 后保留各自页码，键盘可返回动画首页", async () => {
  await setup("animation", "succeeded", false, [
    historyRecord("precheck", "precheck", 10),
    ...[10, 11, 12, 13].map((day) => historyRecord(`animation-${day}`, "animation", day)),
  ]);
  const animations = screen.getByRole("region", { name: "动画检测记录" });
  const previous = within(animations).getByRole("button", { name: "上一页记录" });
  expect(previous).toBeDisabled();
  await userEvent.click(within(animations).getByRole("button", { name: "下一页记录" }));
  expect(within(animations).getAllByRole("article")).toHaveLength(1);
  expect(within(animations).getByRole("article")).toHaveAccessibleName("自定义检测 animation-10");
  const tabs = within(screen.getByRole("tablist", { name: "自定义检测记录类型" }));
  await userEvent.click(tabs.getByRole("tab", { name: "前置检测" }));
  expect(screen.getByRole("article", { name: "自定义检测 precheck" })).toBeVisible();
  await userEvent.click(tabs.getByRole("tab", { name: "动画检测" }));
  expect(within(animations).getByRole("article")).toHaveAccessibleName("自定义检测 animation-10");
  expect(within(animations).getByRole("button", { name: "下一页记录" })).toBeDisabled();
  previous.focus();
  await userEvent.keyboard("{Enter}");
  expect(within(animations).getAllByRole("article")[0]).toHaveAccessibleName(
    "自定义检测 animation-13",
  );
});

it("某一类没有记录时可切换查看空状态，再切回已有记录", async () => {
  await setup("animation", "succeeded");
  const tabs = within(screen.getByRole("tablist", { name: "自定义检测记录类型" }));
  await userEvent.click(tabs.getByRole("tab", { name: "前置检测" }));
  expect(screen.getByText("暂无前置检测记录")).toBeVisible();
  expect(screen.queryByRole("article")).not.toBeInTheDocument();
  await userEvent.click(tabs.getByRole("tab", { name: "动画检测" }));
  expect(screen.getByRole("article")).toBeVisible();
});

it.each([1, 2, 3])("当前页有 %i 条记录时桌面保留三列，手机使用单列", async (count) => {
  await setup(
    "animation",
    "succeeded",
    false,
    Array.from({ length: count }, (_, index) =>
      historyRecord(`animation-${index}`, "animation", 10 + index),
    ),
  );
  const cards = screen.getAllByRole("article");
  expect(cards).toHaveLength(count);
  expect(cards[0].parentElement).toHaveClass("grid-cols-1", "md:grid-cols-3");
});

it("自定义接口同批多个模型可切换查看各自结果和原接口", async () => {
  const completedAt = "2026-10-13T00:00:00Z";
  const row = {
    ...source,
    account_name: "自定义接口",
    mode: "animation",
    status: "failed",
    error: "首个模型不可用",
    request_id: "first-request",
    duration_ms: 10,
    completed_at: completedAt,
  };
  const multi: Task = {
    id: "multi-custom",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "failed",
    progress: 100,
    message: "检测完成",
    created_at: completedAt,
    updated_at: completedAt,
    result: {
      account_ids: [source.account_id],
      targets: [{ ...source, models: [source.model, "second-model"] }],
      animations: [
        row,
        { ...row, model: "second-model", request_id: "second-request", error: "第二个模型不可用" },
      ],
    },
  };
  await setup("animation", "failed", false, [multi]);
  const card = screen.getByRole("article", { name: "自定义检测 requested-model" });
  const select = within(card).getByRole("group", { name: "结果模型" });
  await userEvent.setup().click(within(select).getByRole("button", { name: "下一个模型结果" }));
  expect(select).toHaveTextContent("second-model");
  expect(card).toHaveAccessibleName("自定义检测 second-model");
  expect(within(card).getByText("第二个模型不可用")).toBeVisible();
  expect(card).toHaveTextContent(endpoint);
  expect(within(card).queryByText("首个模型不可用")).not.toBeInTheDocument();
});
