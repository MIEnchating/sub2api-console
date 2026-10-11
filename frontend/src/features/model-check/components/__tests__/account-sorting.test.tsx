import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { AccountStatus } from "@/api";
import { account } from "@/features/accounts/__tests__/fixtures";
import { AnimationCheckPanel } from "../animation-check-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

const accounts: AccountStatus[] = [
  {
    ...account,
    id: "1",
    name: "手动账号",
    platform: "openai",
    priority: 10,
    manual_priority: 1,
    weight: 2.5,
  },
  { ...account, id: "2", name: "自动账号", platform: "openai", priority: 2, weight: 10 },
  { ...account, id: "3", name: "缺失参数账号", platform: "openai", priority: null, weight: null },
];
function setup(rows = accounts): QueryClient {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["accounts"], rows);
  client.setQueryData(["policy"], {});
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], []);
  for (const dictionary of ["group", "platform"])
    client.setQueryData(["dictionaries", dictionary], { items: [] });
  render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return client;
}

it.each(["账号检测", "前置检测"])(
  "%s 展示实际优先级与调度权重，零值保留且缺失不补零",
  async (tab) => {
    const rows = [...accounts, { ...account, id: "4", name: "零值账号", priority: 0, weight: 0 }];
    setup(rows);
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: tab }));
    const manual = within(screen.getByRole("article", { name: "账号 手动账号" }));
    expect(manual.getByText("手动控制 #1")).toBeVisible();
    const metrics = within(manual.getByRole("group", { name: "账号调度参数" }));
    expect(metrics.getByText("10", { exact: true })).toBeVisible();
    expect(metrics.getByText("2.5", { exact: true })).toBeVisible();
    const missing = within(screen.getByRole("article", { name: "账号 缺失参数账号" })).getByRole(
      "group",
      { name: "账号调度参数" },
    );
    expect(within(missing).getAllByText("—")).toHaveLength(2);
    const zero = within(screen.getByRole("article", { name: "账号 零值账号" })).getByRole("group", {
      name: "账号调度参数",
    });
    expect(within(zero).getAllByText("0", { exact: true })).toHaveLength(2);
  },
);

it.each([
  { label: "优先级：从小到大", names: ["自动账号", "手动账号", "缺失参数账号"] },
  { label: "优先级：从大到小", names: ["手动账号", "自动账号", "缺失参数账号"] },
  { label: "权重：从小到大", names: ["手动账号", "自动账号", "缺失参数账号"] },
  { label: "权重：从大到小", names: ["自动账号", "手动账号", "缺失参数账号"] },
])("选择 $label 时按数值排序并把缺失值放在末尾，切换标签保留排序及勾选", async (scenario) => {
  setup();
  const user = userEvent.setup();
  await user.click(
    within(screen.getByRole("article", { name: "账号 手动账号" })).getByRole("checkbox"),
  );
  await user.click(screen.getByRole("combobox", { name: "账号排序" }));
  await user.click(screen.getByRole("option", { name: scenario.label }));
  expect(screen.getAllByRole("article").map((card) => card.getAttribute("aria-label"))).toEqual(
    scenario.names.map((name) => `账号 ${name}`),
  );
  await user.click(screen.getByRole("tab", { name: "前置检测" }));
  expect(screen.getByRole("combobox", { name: "账号排序" })).toHaveTextContent(scenario.label);
  expect(
    within(screen.getByRole("article", { name: "账号 手动账号" })).getByRole("checkbox"),
  ).toBeChecked();
  expect(screen.getAllByRole("article").map((card) => card.getAttribute("aria-label"))).toEqual(
    scenario.names.map((name) => `账号 ${name}`),
  );
  await user.click(screen.getByRole("button", { name: "重置筛选" }));
  expect(screen.getByRole("combobox", { name: "账号排序" })).toHaveTextContent("默认排序");
  expect(screen.getAllByRole("article").map((card) => card.getAttribute("aria-label"))).toEqual(
    accounts.map((row) => `账号 ${row.name}`),
  );
});

it("在第二页调整排序后回到首页，分页前排序且账号刷新后重新排序", async () => {
  const rows = Array.from({ length: 13 }, (_, index) => ({
    ...account,
    id: String(index + 1),
    name: `账号${index + 1}`,
    priority: index + 1,
    weight: index + 1,
  }));
  const client = setup(rows);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getAllByRole("article")).toHaveLength(1);
  const sort = screen.getByRole("combobox", { name: "账号排序" });
  sort.focus();
  await user.keyboard("{Enter}");
  await user.click(screen.getByRole("option", { name: "权重：从大到小" }));
  expect(screen.getAllByRole("article")).toHaveLength(12);
  expect(screen.getAllByRole("article")[0]).toHaveAccessibleName("账号 账号13");
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  act(() =>
    client.setQueryData(
      ["accounts"],
      rows.map((row) => (row.id === "1" ? { ...row, weight: 100 } : row)),
    ),
  );
  await waitFor(() => expect(screen.getAllByRole("article")[0]).toHaveAccessibleName("账号 账号1"));
  expect(
    within(screen.getAllByRole("article")[0]).getByRole("group", { name: "账号调度参数" }),
  ).toHaveTextContent("100");
});

it("筛选无结果或接口返回空列表时仍可选择排序且不启用检测", async () => {
  setup([]);
  const user = userEvent.setup();
  await user.click(screen.getByRole("combobox", { name: "账号排序" }));
  await user.click(screen.getByRole("option", { name: "优先级：从小到大" }));
  expect(screen.getByText("没有匹配的账号")).toBeVisible();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
});
