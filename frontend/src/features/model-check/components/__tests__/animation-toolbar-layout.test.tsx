import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
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

it("账号筛选和模型设置共享可换行的设置行，模型宽度受容器约束", () => {
  setup();
  const settings = screen.getByRole("group", { name: "动画筛选与模型" });
  expect(settings).toHaveClass("flex-wrap");
  expect(within(settings).getByRole("group", { name: "动画账号筛选" })).toBeVisible();
  const fields = within(settings).getByRole("group", { name: "动画模型参数" });
  expect(fields).toHaveClass("w-full", "sm:w-[27rem]", "max-w-full");
  expect(within(fields).getByRole("combobox", { name: "检测模型" })).toBeVisible();
  expect(within(fields).getByRole("spinbutton", { name: "请求超时（秒）" })).toBeVisible();
});

it("前置检测和动画启动共享操作行，筛选控件保持独立", () => {
  setup();
  const operations = screen.getByRole("group", { name: "动画检测操作" });
  expect(within(operations).getByRole("group", { name: "前置检测操作" })).toBeVisible();
  expect(within(operations).getByRole("button", { name: "选择前 20 个账号" })).toBeVisible();
  expect(within(operations).getByRole("button", { name: "清空选择" })).toBeVisible();
  expect(within(operations).getByRole("button", { name: /开始检测/ })).toBeVisible();
  expect(within(operations).queryByRole("combobox", { name: "检测模型" })).not.toBeInTheDocument();
});

it("窄屏卡片列表完整展开并由表单统一滚动，桌面保留列表内部滚动", () => {
  setup();
  const region = screen.getByRole("region", { name: "动画账号卡片" });
  expect(region).toHaveClass("flex-none", "shrink-0", "overflow-visible", "md:overflow-y-auto");
  expect(region.closest("form")).toHaveClass("overflow-y-auto", "md:overflow-hidden");
  expect(screen.getByRole("group", { name: "动画账号筛选" })).toHaveClass("flex-wrap");
});
