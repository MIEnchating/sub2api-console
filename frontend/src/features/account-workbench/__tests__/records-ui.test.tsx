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
it("检测未通过的已导入账号显示手动启用入口，确认后携带批次版本和所选 ID", async () => {
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
  expect(screen.getAllByRole("button", { name: "启用" })).toHaveLength(2);
  const user = userEvent.setup();
  await user.click(screen.getAllByRole("button", { name: "启用" })[0]);
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
  for (const button of screen.getAllByRole("button", { name: "启用" }))
    expect(button).toBeDisabled();
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

it("检测请求出错时显示原因并提供手动启用入口", () => {
  mount(vi.fn(), {
    ...run,
    items: [
      { ...run.items[1], check: { verdict: "ERROR", error: "OAuth 检测请求失败（HTTP 429）" } },
    ],
  });
  expect(screen.getByText(/OAuth 检测请求失败（HTTP 429）/)).toBeVisible();
  expect(screen.getByText("检测出错")).toBeVisible();
  expect(screen.getByRole("button", { name: "启用" })).toBeEnabled();
});

it("部分检测请求失败时保留原因和手动启用入口", () => {
  mount(vi.fn(), {
    ...run,
    items: [{ ...run.items[0], check: { verdict: "INCONCLUSIVE", error: "请求超时" } }],
  });
  expect(screen.getByText(/请求超时/)).toBeVisible();
  expect(screen.getByRole("button", { name: "启用" })).toBeEnabled();
});

it("检测正常完成且更接近 Luna 时显示具体结论，历史隔离项允许启用", () => {
  mount(vi.fn(), {
    ...run,
    items: [{ ...run.items[0], check: { verdict: "LUNA_LIKE", error: null } }],
  });
  expect(screen.getByText("更接近 Luna")).toBeVisible();
  expect(screen.getByRole("button", { name: "启用" })).toBeEnabled();
});

it("导入前校验失败且没有站点账号时不提供手动启用入口", () => {
  mount(vi.fn(), {
    ...run,
    items: [
      { ...run.items[1], status: "failed", account_id: undefined, message: "访问令牌已过期" },
    ],
  });
  expect(screen.getByText("访问令牌已过期")).toBeVisible();
  expect(screen.queryByRole("button", { name: "启用" })).not.toBeInTheDocument();
});

it("手动启用成功后显示启用结果并可打开原检测错误报告", async () => {
  mount(vi.fn(), {
    ...run,
    status: "completed",
    items: [
      {
        ...run.items[1],
        status: "completed",
        manual_enabled: true,
        message: "已启用（保留原检测结论）",
        check: { verdict: "ERROR", error: "请求超时" },
      },
    ],
  });
  expect(screen.getByText("已启用（保留原检测结论）")).toBeVisible();
  expect(screen.getByText("检测出错")).toBeVisible();
  expect(screen.getByRole("button", { name: "检测详情" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "启用" })).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "检测详情" }));
  expect(within(screen.getByRole("dialog")).getByText("请求超时")).toBeVisible();
});
