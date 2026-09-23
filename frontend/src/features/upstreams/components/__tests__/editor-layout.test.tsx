import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { api, type UpstreamConfiguration } from "@/api";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function configuration(): UpstreamConfiguration {
  return {
    upstream_id: "up_example",
    host: "api.example.test",
    name: "测试上游",
    base_url: "https://api.example.test",
    account_base_url: "https://api.example.test/v1",
    upstream_type: "sub2api",
    auth_mode: "sub2api_user_token",
    recharge_rate: "1",
    raw_balance: "24",
    balance: "24",
    has_access_token: true,
    has_refresh_token: false,
    has_admin_key: false,
    has_user_id: false,
    headers: {},
    header_names: [],
    cookie_names: [],
    groups: [],
  };
}

function renderEditor(data = configuration()): { onOpenChange: ReturnType<typeof vi.fn> } {
  client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
  client.setQueryData(["upstream-configuration", data.host], data);
  client.setQueryData(["auth-recovery-config"], { vault_entries: [] });
  client.setQueryData(["dictionaries", "upstream_type"], { items: [] });
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <UpstreamEditDialog host={data.host} onOpenChange={onOpenChange} onSaved={() => undefined} />
    </QueryClientProvider>,
  );
  return { onOpenChange };
}

it("共享并发条左右内容高度不同时，分配开关在单元格内垂直居中", async () => {
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue({
    revision: "v1",
    target_id: "up_example",
    upstream_id: "up_example",
    override: null,
    selected: true,
    effective: true,
    global_enabled: true,
    source: "policy",
  });
  renderEditor();
  await screen.findByRole("switch", { name: "上游共享并发分配" });
  const allocation = screen.getByRole("region", { name: "上游共享并发分配" });
  expect(allocation.parentElement).toHaveClass("grid", "items-center");
  expect(allocation.parentElement).toHaveClass("border-t", "lg:border-t-0", "lg:border-l");
});

it("默认配置页展示紧凑并发条和两列字段，账号关系使用独立页签", () => {
  renderEditor({
    ...configuration(),
    concurrency_limit: 1000,
    concurrency_status: "known",
    allocated_concurrency: 800,
    target_concurrency: 800,
  });

  const concurrency = screen.getByRole("region", { name: "共享并发" });
  expect(within(concurrency).getByLabelText("用户上限")).toHaveTextContent("1000");
  expect(within(concurrency).getByLabelText("已配置并发")).toHaveTextContent("800");
  expect(within(concurrency).queryByLabelText("目标并发")).not.toBeInTheDocument();
  expect(screen.getByRole("region", { name: "连接设置" })).toBeVisible();
  expect(screen.getByRole("region", { name: "鉴权配置" })).toBeVisible();
  expect(screen.getByRole("textbox", { name: "名称" })).toHaveValue("测试上游");
  expect(screen.getByRole("combobox", { name: "平台" })).toBeVisible();
  expect(screen.getByRole("combobox", { name: "鉴权方式" })).toBeVisible();
  expect(screen.getByRole("tab", { name: "配置" })).toHaveAttribute("aria-selected", "true");
  expect(screen.getByRole("tabpanel", { name: "配置" })).toBeVisible();
  expect(screen.queryByRole("region", { name: "当前上游账号" })).not.toBeInTheDocument();
  expect(concurrency).toHaveClass("flex", "flex-wrap");
  expect(concurrency).not.toHaveClass("border", "rounded-xl");
  expect(screen.getByRole("region", { name: "连接设置" })).not.toHaveClass("border", "rounded-xl");
  expect(screen.getByRole("dialog").querySelectorAll('[data-slot="field-error"]')).toHaveLength(0);
});

it("长地址和余额在窄弹窗内可收缩，只有正文垂直滚动且保存取消留在页脚", () => {
  renderEditor({
    ...configuration(),
    name: "很长的上游名称".repeat(20),
    balance: "1234567890".repeat(8),
  });
  const dialog = screen.getByRole("dialog", { name: "编辑上游" });
  const body = dialog.querySelector('[data-slot="dialog-body"]');
  expect(body).toHaveClass("min-w-0", "overflow-y-auto", "overflow-x-clip");
  expect(screen.getByTestId("upstream-editor-sections")).toHaveClass("grid-cols-1", "min-w-0");
  expect(screen.getByTestId("upstream-connection-fields")).toHaveClass("sm:grid-cols-2");
  expect(screen.getByLabelText("换算后余额")).toHaveClass("break-all");
  for (const name of ["保存并重算", "取消"]) {
    expect(
      screen.getByRole("button", { name }).closest('[data-slot="dialog-footer"]'),
    ).not.toBeNull();
  }
});

it("并发后台刷新时更新额度且保留编辑中的名称和凭据", async () => {
  const data = {
    ...configuration(),
    concurrency_limit: 10,
    concurrency_status: "known" as const,
    allocated_concurrency: 12,
  };
  renderEditor(data);
  const user = userEvent.setup();
  await user.clear(screen.getByRole("textbox", { name: "名称" }));
  await user.type(screen.getByRole("textbox", { name: "名称" }), "正在编辑");
  await user.type(screen.getByLabelText("Token"), "private-draft");

  await act(async () => {
    client.setQueryData(["upstream-configuration", data.host], { ...data, concurrency_limit: 20 });
  });

  await waitFor(() => expect(screen.getByLabelText("用户上限")).toHaveTextContent("20"));
  expect(screen.queryByText("已超上限")).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "名称" })).toHaveValue("正在编辑");
  expect(screen.getByLabelText("Token")).toHaveValue("private-draft");
});

it("未读取并发时展示同步指引，并可用Escape关闭弹窗", async () => {
  const state = renderEditor();

  const concurrency = screen.getByRole("region", { name: "共享并发" });
  expect(within(concurrency).getByLabelText("用户上限")).toHaveTextContent("未读取");
  expect(concurrency).not.toHaveTextContent("通过「同步上游」读取用户并发上限。");
  await waitFor(() => expect(screen.getByRole("heading", { name: "编辑上游" })).toHaveFocus());
  await userEvent.setup().keyboard("{Escape}");

  expect(state.onOpenChange.mock.lastCall?.[0]).toBe(false);
});

it("用键盘切换账号页签后返回配置，名称和凭据草稿保持且焦点跟随页签", async () => {
  renderEditor();
  const user = userEvent.setup();
  await user.clear(screen.getByRole("textbox", { name: "名称" }));
  await user.type(screen.getByRole("textbox", { name: "名称" }), "编辑中的上游");
  await user.type(screen.getByLabelText("Token"), "private-draft");
  const configTab = screen.getByRole("tab", { name: "配置" });
  configTab.focus();

  await user.keyboard("{ArrowRight}");

  expect(screen.getByRole("tab", { name: /关联账号/ })).toHaveFocus();
  expect(screen.getByRole("tabpanel", { name: /关联账号/ })).toBeVisible();
  expect(screen.getByText("当前上游暂无绑定账号")).toBeVisible();
  expect(screen.queryByRole("textbox", { name: "名称" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "保存并重算" })).not.toBeInTheDocument();
  await user.keyboard("{ArrowLeft}");

  expect(configTab).toHaveFocus();
  expect(screen.getByRole("textbox", { name: "名称" })).toHaveValue("编辑中的上游");
  expect(screen.getByLabelText("Token")).toHaveValue("private-draft");
});

it("打开缓存的 New API 配置时首帧也不请求共享额度，未保存的平台切换不触发读取", async () => {
  const read = vi
    .spyOn(api, "upstreamAllocationSetting")
    .mockRejectedValue(new Error("仅 Sub2API 上游支持共享并发分配"));
  renderEditor({ ...configuration(), upstream_type: "newapi", auth_mode: "newapi_admin_key" });
  expect(screen.getByRole("combobox", { name: "平台" })).toHaveTextContent("New API");
  expect(read).not.toHaveBeenCalled();
  expect(screen.queryByRole("switch", { name: "上游共享并发分配" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("combobox", { name: "平台" }));
  await userEvent.click(screen.getByRole("option", { name: "Sub2API" }));
  expect(read).not.toHaveBeenCalled();
});

it("New API Session 模式只展示 Session Cookie 和用户 ID 凭据", () => {
  renderEditor({
    ...configuration(),
    upstream_type: "newapi",
    auth_mode: "newapi_session",
    has_access_token: false,
    has_user_id: true,
    cookie_names: ["session"],
  });

  expect(screen.getByRole("combobox", { name: "鉴权方式" })).toHaveTextContent(
    "Session Cookie + 用户 ID",
  );
  expect(screen.getByLabelText("Session Cookie")).toHaveAttribute("type", "password");
  expect(screen.getByLabelText("Session Cookie")).toHaveAttribute(
    "placeholder",
    "已配置，留空则不修改",
  );
  expect(screen.getByLabelText("User ID")).toBeVisible();
  expect(screen.queryByLabelText("Admin Key")).not.toBeInTheDocument();
  expect(screen.queryByLabelText("Token")).not.toBeInTheDocument();
});
