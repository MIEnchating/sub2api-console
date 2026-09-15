import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Task, WorkbenchHistoryQuery } from "@/api";
import { WorkbenchHistoryQuery as HistoryQuery } from "../components/workbench-history-query";
import { WorkbenchGlobalCancel } from "../components/workbench-global-cancel";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const prior: Task = {
  id: "older-than-100",
  skill: "account-workbench",
  operation: "account-workbench-import",
  status: "succeeded",
  message: "历史账号",
  progress: 100,
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
  result: {},
};

function mount(element: React.ReactElement): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}

it("完整邮箱批量查询提交规范化邮箱并从服务端读取早期任务", async () => {
  const queries: WorkbenchHistoryQuery[] = [];
  const onSelect = vi.fn();
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (_url, init) => {
      queries.push(JSON.parse(String(init?.body)) as WorkbenchHistoryQuery);
      return Response.json({ items: [prior], total: 1, limit: 50, offset: 0 });
    }),
  );
  mount(<HistoryQuery onSelect={onSelect} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "查询全部历史" }));
  const dialog = screen.getByRole("dialog", { name: "查询全部历史" });
  await within(dialog).findByText("共 1 条 · 第 1 页");
  await user.type(
    within(dialog).getByRole("textbox", { name: "批量查询邮箱" }),
    "Alice@example.com\nbob@example.com\nalice@example.com",
  );
  await user.click(within(dialog).getByRole("button", { name: "查询记录" }));
  await waitFor(() =>
    expect(queries.at(-1)?.emails).toEqual(["alice@example.com", "bob@example.com"]),
  );
  await user.click(
    await within(dialog).findByRole("button", { name: "查看历史任务 older-than-100" }),
  );
  expect(onSelect).toHaveBeenCalledWith(prior);
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});

it("历史分页切换会清空当前页选择并允许读取第 100 条之后记录", async () => {
  const offsets: number[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (_url, init) => {
      const input = JSON.parse(String(init?.body)) as WorkbenchHistoryQuery;
      offsets.push(input.offset);
      return Response.json({
        items: [{ ...prior, id: `page-${input.offset}` }],
        total: 125,
        limit: 50,
        offset: input.offset,
      });
    }),
  );
  mount(<HistoryQuery onSelect={() => undefined} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "查询全部历史" }));
  await user.click(await screen.findByRole("checkbox", { name: "选择历史记录 page-0" }));
  expect(screen.getByRole("button", { name: "删除选中记录（1）" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "下一页" }));
  expect(await screen.findByRole("checkbox", { name: "选择历史记录 page-50" })).not.toBeChecked();
  await user.click(screen.getByRole("button", { name: "下一页" }));
  await screen.findByRole("checkbox", { name: "选择历史记录 page-100" });
  expect(offsets).toEqual([0, 50, 100]);
  expect(screen.getByRole("button", { name: "下一页" })).toBeDisabled();
});

it("无效邮箱显示字段校验且不发送历史查询", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>(async () =>
    Response.json({ items: [], total: 0, limit: 50, offset: 0 }),
  );
  vi.stubGlobal("fetch", fetch);
  mount(<HistoryQuery onSelect={() => undefined} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "查询全部历史" }));
  await screen.findByText("没有匹配的历史记录");
  const field = screen.getByRole("textbox", { name: "批量查询邮箱" });
  await user.type(field, "invalid-email");
  await user.click(screen.getByRole("button", { name: "查询记录" }));
  await screen.findByText(/请填写最多 500 个完整邮箱/);
  expect(field).toHaveAttribute("aria-invalid", "true");
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("读取全部取消范围失败时不能确认并能关闭弹窗", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () => Response.json({ detail: "服务暂不可用" }, { status: 503 })),
  );
  mount(<WorkbenchGlobalCancel />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "取消全部工作台任务" }));
  await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "确认取消这些任务" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "返回" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});

it("读取全部取消范围尚未完成时显示忙碌提示并允许返回", async () => {
  let resolve: (response: Response) => void = () => undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    ),
  );
  mount(<WorkbenchGlobalCancel />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "取消全部工作台任务" }));
  expect(screen.getByRole("status", { name: "正在读取全部活动任务" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(screen.getByRole("button", { name: "确认取消这些任务" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "返回" }));
  resolve(Response.json([prior]));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});
