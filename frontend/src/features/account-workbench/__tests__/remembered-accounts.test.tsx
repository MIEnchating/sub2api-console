import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { RememberedAccountList } from "../components/remembered-account-list";
import { rememberedAccountKey } from "../constants";
import type { RememberedAccount } from "../types";

const account: RememberedAccount = {
  id: "saved-one",
  account_id: "41",
  name: "团队账号",
  email: "owner@example.test",
  plan: "plus",
  login_available: true,
  last_imported_at: "2026-10-11T01:00:00Z",
  revision: 3,
};
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});
function mount(accounts: RememberedAccount[], fetcher: typeof fetch, onStarted = vi.fn()): void {
  vi.stubGlobal("fetch", fetcher);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(rememberedAccountKey, accounts);
  render(
    <QueryClientProvider client={client}>
      <RememberedAccountList onStarted={onStarted} />
      <Toaster />
    </QueryClientProvider>,
  );
}
it("删除保存资料先确认账号，失败保留记录，成功后刷新列表", async () => {
  const writes: unknown[] = [];
  let attempt = 0;
  mount(
    [account],
    vi.fn(async (_input, init) => {
      if (init?.method === "DELETE") {
        writes.push(JSON.parse(String(init.body)));
        attempt++;
        return attempt === 1
          ? Response.json({ detail: "资料版本已变化" }, { status: 409 })
          : new Response(null, { status: 204 });
      }
      return Response.json([]);
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "删除 团队账号" }));
  let dialog = within(screen.getByRole("dialog", { name: "删除已记住账号" }));
  expect(dialog.getByText(/owner@example.test.*#41/)).toBeVisible();
  await user.click(dialog.getByRole("button", { name: "返回" }));
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "删除 团队账号" }));
  dialog = within(screen.getByRole("dialog", { name: "删除已记住账号" }));
  await user.click(dialog.getByRole("button", { name: "确认删除" }));
  expect(await screen.findByText("资料版本已变化")).toBeVisible();
  await waitFor(() => expect(dialog.getByRole("button", { name: "确认删除" })).toBeEnabled());
  await user.click(dialog.getByRole("button", { name: "确认删除" }));
  expect(await screen.findByText("暂无已记住账号")).toBeVisible();
  expect(writes).toEqual([{ revision: 3 }, { revision: 3 }]);
});
it("重新登录先核对资料并预览，确认只提交后端预览版本", async () => {
  const onStarted = vi.fn();
  const requests: Array<{ path: string; body: unknown }> = [];
  mount(
    [account],
    vi.fn(async (input, init) => {
      requests.push({ path: String(input), body: JSON.parse(String(init?.body)) });
      if (String(input).endsWith("/preview"))
        return Response.json({
          id: "preview-one",
          revision: 8,
          expires_at: "2099-01-01T00:00:00Z",
          action: "import",
          check: false,
          promote: true,
          template: null,
          items: [{ id: "row-one", index: 0, kind: "login", email: account.email }],
          errors: [],
          duplicate_count: 0,
        });
      return Response.json({ id: "run-one" });
    }),
    onStarted,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "重新登录 owner@example.test" }));
  const preview = await screen.findByRole("dialog", { name: "确认本批导入" });
  expect(requests).toHaveLength(1);
  await user.click(within(preview).getByRole("button", { name: "确认并开始" }));
  await waitFor(() => expect(onStarted).toHaveBeenCalledOnce());
  expect(requests).toEqual([
    { path: "/api/account-workbench/remembered-accounts/saved-one/preview", body: { revision: 3 } },
    { path: "/api/account-workbench/runs", body: { id: "preview-one", revision: 8 } },
  ]);
});
it("关闭重新登录核对弹窗会取消读取，迟到结果不能重开预览", async () => {
  let finish!: (response: Response) => void;
  let signal: AbortSignal | null | undefined;
  mount(
    [account],
    vi.fn((_input, init) => {
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "重新登录 owner@example.test" }));
  expect(screen.getByRole("status", { name: "正在核对账号与当前配置" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(signal?.aborted).toBe(true);
  await act(async () => finish(Response.json({ id: "late-preview" })));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("仅 JSON 导入记录禁用重新登录，长账号文案换行并允许逐页删除", async () => {
  const records = Array.from({ length: 21 }, (_, index) => ({
    ...account,
    id: `saved-${index}`,
    name: `长名称${"账号".repeat(35)}-${index}`,
    email: `${index}@example.test`,
    login_available: false,
  }));
  mount(records, vi.fn());
  const user = userEvent.setup();
  const table = screen.getByRole("table", { name: "已记住账号列表" });
  expect(within(table).getAllByRole("row")).toHaveLength(21);
  expect(screen.getByRole("button", { name: "重新登录 0@example.test" })).toBeDisabled();
  const cell = within(table).getByText(records[0].name).closest("td");
  expect(cell).toHaveClass("whitespace-normal", "wrap-anywhere");
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(within(table).getAllByRole("row")).toHaveLength(2);
  expect(screen.getByRole("button", { name: `删除 ${records[20].name}` })).toBeEnabled();
});

it("已记住账号的工具栏不重复显示分页账号数量", () => {
  mount([account], vi.fn());
  expect(
    within(screen.getByRole("group", { name: "工作台操作" })).queryByText(/个账号/),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("navigation", { name: "表格分页" })).toHaveTextContent(/共\s*1/);
});
