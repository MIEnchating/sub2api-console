import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { NewAPIManagementPage } from "../newapi-management-page";

let client: QueryClient;
const longName = "超长渠道名称".repeat(30);
const workspace = {
  platforms: [
    {
      id: "primary",
      name: "测试",
      base_url: "https://example.test",
      user_id: "1",
      admin_key_configured: true,
      updated_at: "",
    },
  ],
  local_groups: [],
  bindings: [],
  sub2api_base_url: "https://sub.example.test",
};
const snapshot = {
  groups: [],
  models: [],
  unset_models: [],
  tool_prices: [],
  references: [],
  differences: [],
};
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function mount(): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <NewAPIManagementPage view="channels" />
    </QueryClientProvider>,
  );
}

it("渠道页以可滚动表格展示长文本，新增表单按需打开且关闭后移除", async () => {
  const fetch = vi.fn(async (url: RequestInfo | URL) => {
    if (String(url).includes("/channels?"))
      return Response.json({
        items: [
          {
            id: "42",
            name: longName,
            type: 59,
            status: 1,
            groups: ["default"],
            models: ["gpt-5"],
            version: "a".repeat(64),
          },
        ],
        total: 1,
      });
    if (String(url).includes("/refresh")) return Response.json(snapshot);
    if (String(url).includes("/auth-recovery/config")) return Response.json({ vault_entries: [] });
    if (String(url).includes("/dictionaries")) return Response.json({ items: [] });
    return Response.json(workspace);
  });
  vi.stubGlobal("fetch", fetch);
  mount();
  const user = userEvent.setup();
  const table = await screen.findByRole("table", { name: "现有渠道" });
  expect(table).toHaveAttribute("data-action-column", "true");
  expect(table.parentElement).toHaveClass("overflow-auto", "min-h-0", "flex-1");
  expect(screen.getByText(longName)).toHaveClass("truncate");
  await user.keyboard("{Tab}");
  act(() => screen.getByText(longName).focus());
  expect(await screen.findByRole("tooltip")).toHaveTextContent(longName);
  await user.keyboard("{Escape}");
  expect(fetch.mock.calls.some((call) => String(call[0]).includes("/refresh"))).toBe(false);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "新增渠道" }));
  const dialog = await screen.findByRole("dialog", { name: "新增渠道" });
  expect(dialog.querySelector('[data-slot="dialog-body"]')).toHaveClass(
    "overflow-y-auto",
    "min-h-0",
  );
  expect(await within(dialog).findByRole("button", { name: "自定义账号密码" })).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: "自定义账号密码" }));
  await user.type(within(dialog).getByLabelText("登录邮箱"), "temporary@example.test");
  await user.click(within(dialog).getByRole("button", { name: "取消" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "新增渠道" }));
  await user.click(await screen.findByRole("button", { name: "自定义账号密码" }));
  expect(screen.getByLabelText("登录邮箱")).toHaveValue("");
});

it("新增弹窗配置读取失败提供重试和取消，已有渠道仍可浏览", async () => {
  let failed = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) => {
      if (String(url).includes("/channels?")) return Response.json({ items: [], total: 0 });
      if (String(url).includes("/refresh"))
        return failed
          ? Response.json({ detail: "分组暂时不可用" }, { status: 503 })
          : Response.json(snapshot);
      if (String(url).includes("/auth-recovery/config"))
        return Response.json({ vault_entries: [] });
      if (String(url).includes("/dictionaries")) return Response.json({ items: [] });
      return Response.json(workspace);
    }),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "新增渠道" }));
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("button", { name: "自定义账号密码" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  failed = false;
  await user.click(retry);
  expect(await screen.findByRole("button", { name: "自定义账号密码" })).toBeVisible();
});
