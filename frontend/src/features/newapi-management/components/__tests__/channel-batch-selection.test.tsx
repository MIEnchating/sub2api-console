import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { NewAPIChannel } from "@/api";
import { ExistingChannels } from "../existing-channels";

const items: NewAPIChannel[] = [
  {
    id: "41",
    name: "标准",
    type: 59,
    status: 1,
    groups: ["default"],
    models: ["gpt-5", "gpt-5-mini"],
    version: "a".repeat(64),
  },
  {
    id: "42",
    name: "高级",
    type: 59,
    status: 1,
    groups: ["vip"],
    models: ["gpt-5"],
    version: "b".repeat(64),
  },
];
const task = {
  id: "batch-1",
  operation: "newapi-channel-models",
  skill: "newapi",
  status: "queued",
  progress: 0,
  message: "等待批量更新渠道模型",
  result: {},
  created_at: "",
  updated_at: "",
};
let client: QueryClient;
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
      <ExistingChannels platformId="primary" />
    </QueryClientProvider>,
  );
}

it("渠道行内、分页与批量操作使用全局描边图标按钮，键盘仍可打开维护弹窗", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ items, total: 2 })),
  );
  mount();
  const user = userEvent.setup();
  const row = await screen.findByRole("row", { name: "渠道 标准（41）" });
  const add = within(row).getByRole("button", { name: "上架模型" });
  for (const button of [
    add,
    within(row).getByRole("button", { name: "下架模型" }),
    screen.getByRole("button", { name: "转到上一页" }),
    screen.getByRole("button", { name: "转到下一页" }),
  ]) {
    expect(button).toHaveClass("size-8", "border-border");
  }
  await user.click(screen.getByRole("checkbox", { name: "全选当前筛选渠道" }));
  for (const name of ["批量上架模型", "批量下架模型"]) {
    expect(screen.getByRole("button", { name })).toHaveClass("size-8", "border-border");
  }
  add.focus();
  await user.keyboard("{Enter}");
  expect(screen.getByRole("dialog", { name: "上架模型" })).toBeVisible();
});

it("筛选后全选只选匹配项，跨页保留选择并显示部分选中状态", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) =>
      Response.json({
        items:
          new URL(String(url), "http://localhost").searchParams.get("page") === "1"
            ? [{ ...items[0], id: "43", name: "第三渠道" }]
            : items,
        total: 51,
      }),
    ),
  );
  mount();
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 标准（41）" });
  await user.type(screen.getByRole("textbox", { name: "搜索本页渠道、分组或模型" }), "高级");
  await user.click(screen.getByRole("checkbox", { name: "全选当前筛选渠道" }));
  await user.clear(screen.getByRole("textbox", { name: "搜索本页渠道、分组或模型" }));
  expect(screen.getByRole("checkbox", { name: "全选当前筛选渠道" })).toBePartiallyChecked();
  expect(screen.getByRole("row", { name: "渠道 高级（42）" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  const third = await screen.findByRole("checkbox", { name: "选择渠道 第三渠道（43）" });
  third.focus();
  await user.keyboard(" ");
  expect(screen.getByRole("toolbar", { name: "已选择 2 个渠道的批量操作" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "转到上一页" }));
  expect(await screen.findByRole("checkbox", { name: "选择渠道 高级（42）" })).toBeChecked();
  await user.click(screen.getByRole("button", { name: "清空选择" }));
  expect(screen.queryByRole("toolbar")).not.toBeInTheDocument();
});

it("多选渠道上架先展示各渠道影响，确认只创建一次后台任务并展示部分失败", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
      if (options?.method === "POST") {
        writes.push(JSON.parse(String(options.body)));
        return Response.json(task);
      }
      if (String(url).includes("/tasks/"))
        return Response.json({
          ...task,
          status: "partial",
          progress: 100,
          message: "1 个渠道成功，1 个渠道失败",
          result: {
            items: [
              { channel_id: "41", status: "succeeded", message: "已更新" },
              { channel_id: "42", status: "failed", message: "版本变化，请重新核对" },
            ],
          },
        });
      return Response.json({ items, total: 2 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("checkbox", { name: "全选当前筛选渠道" }));
  await user.click(screen.getByRole("button", { name: "批量上架模型" }));
  await user.type(screen.getByLabelText("上架模型名称"), "gpt-5-mini,gpt-5-nano");
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  const preview = screen.getByRole("group", { name: "渠道影响范围" });
  expect(preview).toHaveTextContent("2 → 3");
  expect(preview).toHaveTextContent("1 → 3");
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "确认上架模型" }));
  expect(await screen.findByText("1 个渠道成功，1 个渠道失败")).toBeVisible();
  expect(
    within(screen.getByRole("group", { name: "渠道执行结果" })).getByText("版本变化，请重新核对"),
  ).toBeVisible();
  expect(writes).toEqual([
    {
      action: "add",
      models: ["gpt-5-mini", "gpt-5-nano"],
      channels: items.map((item) => ({ id: item.id, version: item.version })),
    },
  ]);
});

it("批量下架中任一渠道会被清空时阻止提交，修改选择后可预览", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ items, total: 2 })),
  );
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("checkbox", { name: "全选当前筛选渠道" }));
  await user.click(screen.getByRole("button", { name: "批量下架模型" }));
  await user.click(screen.getByRole("checkbox", { name: "gpt-5" }));
  expect(screen.getByRole("alert")).toHaveTextContent("高级");
  expect(screen.getByRole("button", { name: "预览变更" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "清空" }));
  await user.type(screen.getByRole("textbox", { name: "搜索下架模型" }), "mini");
  await user.click(screen.getByRole("button", { name: "全选结果" }));
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  expect(screen.getByRole("group", { name: "渠道影响范围" })).toHaveTextContent("变更 0 个");
  expect(screen.getByRole("button", { name: "确认下架模型" })).toBeEnabled();
});

it("任务进度读取失败时保留任务入口且不重新提交渠道变更", async () => {
  const fetch = vi.fn(async (url: RequestInfo | URL, options?: RequestInit) => {
    if (options?.method === "POST") return Response.json(task);
    if (String(url).includes("/tasks/"))
      return Response.json({ detail: "读取失败" }, { status: 500 });
    return Response.json({ items, total: 2 });
  });
  vi.stubGlobal("fetch", fetch);
  mount();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("checkbox", { name: "全选当前筛选渠道" }));
  await user.click(screen.getByRole("button", { name: "批量上架模型" }));
  await user.type(screen.getByLabelText("上架模型名称"), "gpt-5-nano");
  await user.click(screen.getByRole("button", { name: "预览变更" }));
  await user.click(screen.getByRole("button", { name: "确认上架模型" }));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeVisible();
  await user.click(screen.getByText("关闭", { selector: "button" }));
  expect(screen.getByRole("button", { name: "查看批量任务" })).toBeVisible();
  expect(screen.getByRole("button", { name: "批量上架模型" })).toBeDisabled();
  expect(fetch.mock.calls.filter((call) => call[1]?.method === "POST")).toHaveLength(1);
});

it("渠道分页提供全局页码和每页行数，切换容量回到第一页并保留渠道选择", async () => {
  const requests: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) => {
      requests.push(String(url));
      return Response.json({ items, total: 120 });
    }),
  );
  mount();
  const user = userEvent.setup();
  const pagination = await screen.findByRole("navigation", { name: "表格分页" });
  expect(pagination).toHaveTextContent("120");
  await user.click(screen.getByRole("checkbox", { name: "选择渠道 标准（41）" }));
  await user.click(within(pagination).getByRole("button", { name: "转到第 2 页" }));
  await waitFor(() => expect(requests.at(-1)).toContain("page=1"));
  await user.click(within(pagination).getByRole("combobox", { name: "每页行数" }));
  await user.click(await screen.findByRole("option", { name: "20" }));
  await waitFor(() => expect(requests.at(-1)).toContain("page=0&page_size=20"));
  expect(await screen.findByRole("checkbox", { name: "选择渠道 标准（41）" })).toBeChecked();
  expect(within(pagination).getByRole("button", { name: "转到第一页" })).toBeDisabled();
});

it("渠道列表为空时在表格内显示共享空状态并保留全局分页", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ items: [], total: 0 })),
  );
  mount();
  const table = await screen.findByRole("table", { name: "现有渠道" });
  expect(within(table).getByText("暂无渠道")).toHaveAttribute("data-slot", "table-empty-state");
  expect(screen.getByRole("navigation", { name: "表格分页" })).toBeVisible();
});

it("翻页读取中保留原表格并禁用分页及渠道操作，返回后显示新页", async () => {
  let complete: (response: Response) => void = () => undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL) => {
      if (new URL(String(url), "http://localhost").searchParams.get("page") === "1")
        return new Promise<Response>((resolve) => {
          complete = resolve;
        });
      return Response.json({ items, total: 51 });
    }),
  );
  mount();
  const user = userEvent.setup();
  await screen.findByRole("row", { name: "渠道 标准（41）" });
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByRole("row", { name: "渠道 标准（41）" })).toBeVisible();
  expect(screen.getByRole("combobox", { name: "每页行数" })).toBeDisabled();
  expect(screen.getByRole("checkbox", { name: "选择渠道 标准（41）" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  complete(Response.json({ items: [{ ...items[0], id: "43", name: "第三渠道" }], total: 51 }));
  expect(await screen.findByRole("row", { name: "渠道 第三渠道（43）" })).toBeVisible();
  expect(screen.getByRole("combobox", { name: "每页行数" })).toBeEnabled();
});
