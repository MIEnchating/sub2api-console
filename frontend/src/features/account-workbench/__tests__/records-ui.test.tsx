import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { RunList } from "../components/run-list";
import { runKeys } from "../constants";
import type { WorkbenchRun } from "../types";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
const run: WorkbenchRun = {
  id: "run-one",
  revision: 4,
  task_id: "task-one",
  action: "import",
  status: "needs_attention",
  created_at: "2026-09-17T10:00:00Z",
  updated_at: "2026-09-17T10:01:00Z",
  expires_at: "2099-01-01T00:00:00Z",
  duplicate_count: 0,
  items: [
    {
      id: "inconclusive",
      index: 0,
      kind: "codex_json",
      name: "",
      email: "review@example.test",
      plan: "",
      identity_source: "official_signature",
      status: "review",
      message: "检测未通过",
      account_id: "41",
      template_name: "默认配置",
      check: { verdict: "INCONCLUSIVE" },
    },
    {
      id: "failed",
      index: 1,
      kind: "codex_json",
      name: "",
      email: "failed@example.test",
      plan: "",
      identity_source: "official_signature",
      status: "review",
      message: "检测未通过",
      account_id: "42",
      template_name: "默认配置",
      check: { verdict: "ERROR" },
    },
  ],
};
function mount(fetcher: typeof fetch, record: WorkbenchRun = run): void {
  vi.stubGlobal("fetch", fetcher);
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(runKeys.list, [record]);
  render(
    <QueryClientProvider client={client}>
      <RunList />
    </QueryClientProvider>,
  );
}
it("只有证据不足的隔离账号显示启用入口，确认后携带批次版本和所选 ID", async () => {
  const writes: Array<{ path: string; body: unknown }> = [];
  mount(
    vi.fn(async (input, init) => {
      if (init?.method === "POST") {
        writes.push({ path: String(input), body: JSON.parse(String(init.body)) });
        return Response.json({});
      }
      return Response.json([run]);
    }),
  );
  expect(screen.getAllByRole("button", { name: "启用" })).toHaveLength(1);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "启用" }));
  expect(writes).toEqual([]);
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "启用所选账号" }),
  );
  await waitFor(() =>
    expect(writes).toEqual([
      {
        path: "/api/account-workbench/runs/run-one/enable",
        body: { revision: 4, ids: ["inconclusive"] },
      },
    ]),
  );
});
it("搜索邮箱缩小批次列表，删除记录先确认且取消不发写请求", async () => {
  const fetcher = vi.fn(async () => Response.json([]));
  mount(fetcher);
  const user = userEvent.setup();
  await user.type(screen.getByRole("searchbox", { name: "搜索处理记录" }), "absent");
  expect(screen.getByText("没有匹配的处理记录")).toBeVisible();
  await user.clear(screen.getByRole("searchbox", { name: "搜索处理记录" }));
  await user.click(screen.getByRole("button", { name: "删除记录" }));
  await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "返回" }));
  expect(fetcher).not.toHaveBeenCalled();
});

it("批次资料到期后保留结果和删除入口，禁用重试、启用与私有导出", () => {
  mount(
    vi.fn(async () => Response.json([])),
    { ...run, expires_at: "2000-01-01T00:00:00Z" },
  );
  expect(screen.getByText("本批登录资料已到期，请重新导入；处理结果仍可查看。")).toBeVisible();
  expect(screen.getByRole("button", { name: "继续 / 重试" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "启用" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "保存私有 JSON" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "删除记录" })).toBeEnabled();
});

it("检测返回不匹配结论时显示中文标签，新增状态不会直接显示协议编码", () => {
  mount(vi.fn(), {
    ...run,
    status: "new_backend_status",
    items: [{ ...run.items[0], status: "new_item_status", check: { verdict: "MISMATCH" } }],
  });
  expect(screen.getByText("检测不匹配")).toBeVisible();
  expect(screen.queryByText("MISMATCH")).not.toBeInTheDocument();
  expect(screen.queryByText("new_backend_status")).not.toBeInTheDocument();
  expect(screen.queryByText("new_item_status")).not.toBeInTheDocument();
});
