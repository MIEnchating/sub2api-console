import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { AnimationCheckPanel } from "../animation-check-panel";

const clients: QueryClient[] = [];
afterEach(() => clients.splice(0).forEach((client) => client.clear()));

function setup(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(
    ["accounts"],
    [{ id: "41", name: "布局账号", groups: [], platform: "openai" }],
  );
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], []);
  render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
}

it.each(["账号检测", "前置检测"])("%s 的模型和超时位于开始检测操作栏内", (tab) => {
  setup();
  fireEvent.click(screen.getByRole("tab", { name: tab }));
  const operations = screen.getByRole("group", {
    name: tab === "账号检测" ? "动画检测操作" : "前置检测操作",
  });
  const fields = within(operations).getByRole("group", { name: "动画模型参数" });
  expect(within(operations).getByRole("button", { name: /开始检测|^前置检测（/ })).toBeVisible();
  expect(fields).toHaveClass("w-full", "sm:w-[36rem]", "max-w-full");
  expect(within(fields).getByRole("combobox", { name: "检测模型" })).toHaveValue("gpt-6-astra");
  expect(within(fields).getByRole("spinbutton", { name: "请求超时（秒）" })).toBeVisible();
});

it("超时标签和输入框同行，窄屏宽度受容器约束", () => {
  setup();
  const fields = screen.getByRole("group", { name: "动画模型参数" });
  expect(fields).toHaveClass("grid-cols-1");
  for (const name of ["检测模型设置", "请求超时设置"]) {
    const field = within(fields).getByRole("group", { name });
    expect(field).toHaveClass("grid", "grid-cols-[auto_minmax(0,1fr)]", "items-center");
    expect(field.querySelector("label")).toHaveClass("whitespace-nowrap");
  }
});

it("动画与前置检测使用独立操作区且保留公共账号选择", () => {
  setup();
  const operations = screen.getByRole("group", { name: "动画检测操作" });
  expect(within(operations).queryByRole("button", { name: /前置检测/ })).not.toBeInTheDocument();
  expect(
    within(operations).queryByRole("button", { name: /选择实时流量/ }),
  ).not.toBeInTheDocument();
  expect(within(operations).getByRole("button", { name: "全选账号" })).toBeVisible();
  expect(within(operations).getByRole("button", { name: "清空选择" })).toBeVisible();
  expect(within(operations).getByRole("button", { name: /开始检测/ })).toBeVisible();
  expect(within(operations).getByRole("combobox", { name: "检测模型" })).toBeVisible();

  fireEvent.click(screen.getByRole("tab", { name: "前置检测" }));
  const precheckOperations = screen.getByRole("group", { name: "前置检测操作" });
  expect(within(precheckOperations).getByRole("button", { name: "全选账号" })).toBeVisible();
  expect(within(precheckOperations).getByRole("button", { name: "清空选择" })).toBeVisible();
  expect(within(precheckOperations).getByRole("button", { name: /^前置检测（/ })).toBeVisible();
  expect(
    within(precheckOperations).queryByRole("button", { name: /开始检测/ }),
  ).not.toBeInTheDocument();
});

it("没有取消或重试操作时，批量设置紧邻开始检测且只保留标准间距", () => {
  setup();
  const operations = screen.getByRole("group", { name: "动画检测操作" });
  const batch = within(operations).getByRole("button", { name: "批量自动检测设置" });
  const start = within(operations).getByRole("button", { name: /开始检测/ });
  expect(batch.parentElement).toBe(start.parentElement);
  expect(batch.nextElementSibling).toBe(start);
  expect(start.parentElement).toHaveClass("flex", "flex-wrap", "gap-2");
});

it("开始检测按钮按内容扩展并保留共享按钮的左右内边距", () => {
  setup();
  const start = screen.getByRole("button", { name: /开始检测/ });
  expect(start).not.toHaveClass("w-40");
  expect(start).toHaveClass("min-w-40", "px-2.5", "h-8");
});

it("提示词设置与账号选择共用下方操作栏，点击后才显示编辑弹窗", () => {
  setup();
  const operations = screen.getByRole("group", { name: "动画检测操作" });
  expect(operations).toHaveClass("flex-wrap", "items-center");
  const trigger = within(operations).getByRole("button", { name: "编辑动画提示词" });
  expect(trigger).toBeVisible();
  expect(screen.queryByRole("textbox", { name: "动画提示词" })).not.toBeInTheDocument();
  fireEvent.click(trigger);
  expect(screen.getByRole("dialog", { name: "动画提示词设置" })).toBeVisible();
});

it("窄屏卡片列表完整展开并由表单统一滚动，桌面保留列表内部滚动", () => {
  setup();
  const region = screen.getByRole("region", { name: "动画账号卡片" });
  expect(region).toHaveClass("flex-none", "shrink-0", "overflow-visible", "md:overflow-y-auto");
  expect(region.closest("form")).toHaveClass("overflow-y-auto", "md:overflow-hidden");
  expect(screen.getByRole("group", { name: "动画账号筛选" })).toHaveClass("flex-wrap");
});
