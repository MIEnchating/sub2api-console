import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ModelCheckPage } from "../model-check-page";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
const summary = {
  id: "old-check",
  skill: "sub2api-model-check",
  operation: "account-model-behavior-check",
  status: "succeeded",
  progress: 100,
  message: "历史检测完成",
  created_at: "2026-10-15T00:00:00Z",
  updated_at: "2026-10-15T00:01:00Z",
};
function setup(history: (url: string) => Promise<Response>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(["accounts"], []);
  client.setQueryData(["model-check-account-statuses"], []);
  const fetcher = vi.fn<typeof fetch>(async (input) => {
    const url = String(input);
    if (url.startsWith("/api/model-checks/history")) return history(url);
    if (url === "/api/tasks/old-check")
      return Response.json({
        ...summary,
        result: {
          tests: [
            {
              checker: "verification",
              account_name: "历史账号",
              claimed_model: "test-model",
              capability: { status: "passed" },
              authenticity: { status: "insufficient_evidence", summary: "缺少官方对照" },
              probes: [
                {
                  id: "arithmetic",
                  label: "计算",
                  round: 1,
                  status: "passed",
                  prompt: "7 × 3",
                  expected: "21",
                  answer: "**21**",
                },
              ],
            },
          ],
        },
      });
    return Response.json([]);
  });
  vi.stubGlobal("fetch", fetcher);
  render(
    <QueryClientProvider client={client}>
      <ModelCheckPage />
    </QueryClientProvider>,
  );
  return fetcher;
}

it("未选择账号也可打开历史并按需查看原始回答，关闭后保留历史列表", async () => {
  const fetcher = setup(async () => Response.json({ items: [summary], has_more: false }));
  expect(fetcher.mock.calls.some((call) => String(call[0]).includes("/history"))).toBe(false);
  await userEvent.click(screen.getByRole("button", { name: "检测历史" }));
  const history = await screen.findByRole("dialog", { name: "检测历史" });
  await within(history).findByText("历史检测完成");
  expect(fetcher.mock.calls.some((call) => String(call[0]).includes("/tasks/old-check"))).toBe(
    false,
  );
  await userEvent.click(within(history).getByRole("button", { name: "查看结果" }));
  const result = await screen.findByRole("dialog", { name: "历史检测结果" });
  expect(await within(result).findByText("历史账号")).toBeVisible();
  await userEvent.click(within(result).getByRole("button", { name: "查看检测证据" }));
  expect(
    await within(screen.getByRole("dialog", { name: "检测证据" })).findByText("**21**"),
  ).toBeVisible();
  await userEvent.keyboard("{Escape}");
  await userEvent.keyboard("{Escape}");
  expect(within(history).getByText("历史检测完成")).toBeVisible();
});

it("历史列表支持分页且空页保留上一页入口", async () => {
  setup(async (url) =>
    Response.json({
      items: url.endsWith("page=1") ? [summary] : [],
      has_more: url.endsWith("page=1"),
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "检测历史" }));
  await screen.findByText("历史检测完成");
  await userEvent.click(screen.getByRole("button", { name: "下一页" }));
  expect(await screen.findByText("暂无模型检测历史")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "上一页" }));
  expect(await screen.findByText("历史检测完成")).toBeVisible();
});

it("历史读取失败提供重试并保留关闭入口", async () => {
  let fail = true;
  setup(async () =>
    fail
      ? Response.json({ error: "暂时无法读取历史" }, { status: 503 })
      : Response.json({ items: [], has_more: false }),
  );
  await userEvent.click(screen.getByRole("button", { name: "检测历史" }));
  const retry = await screen.findByRole("button", { name: "重新读取" });
  fail = false;
  await userEvent.click(retry);
  expect(await screen.findByText("暂无模型检测历史")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "关闭" }));
  expect(screen.queryByRole("dialog", { name: "检测历史" })).not.toBeInTheDocument();
});

it("历史首次加载时保留关闭入口且不显示空记录", async () => {
  let release!: (response: Response) => void;
  setup(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve;
      }),
  );
  await userEvent.click(screen.getByRole("button", { name: "检测历史" }));
  expect(await screen.findByText("正在读取检测历史")).toBeVisible();
  expect(screen.queryByText("暂无模型检测历史")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "关闭" }));
  expect(screen.queryByRole("dialog", { name: "检测历史" })).not.toBeInTheDocument();
  release(Response.json({ items: [], has_more: false }));
});

it("历史结果读取失败可重试且不会重新运行检测", async () => {
  const fetcher = setup(async () => Response.json({ items: [summary], has_more: false }));
  const original = fetcher.getMockImplementation()!;
  let fail = true;
  fetcher.mockImplementation(async (input, init) => {
    if (String(input) === "/api/tasks/old-check" && fail)
      return Response.json({ error: "结果暂时不可用" }, { status: 503 });
    return original(input, init);
  });
  await userEvent.click(screen.getByRole("button", { name: "检测历史" }));
  await userEvent.click(await screen.findByRole("button", { name: "查看结果" }));
  const result = screen.getByRole("dialog", { name: "历史检测结果" });
  const retry = await within(result).findByRole("button", { name: "重新读取" });
  fail = false;
  await userEvent.click(retry);
  expect(await within(result).findByText("历史账号")).toBeVisible();
  expect(fetcher.mock.calls.some((call) => call[1]?.method === "POST")).toBe(false);
});
