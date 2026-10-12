import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { UpstreamsPage } from "@/App";
import { router } from "@/router";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
const host = {
  upstream_id: "upstream-1",
  host: "fixture.example",
  hosts: ["fixture.example"],
  base_url: "https://fixture.example",
  name: "测试上游",
  upstream_type: "sub2api",
  account_count: 1,
  group_count: 1,
  auth_status: "已鉴权",
  raw_balance: "10",
  balance: "10",
  recharge_rate: "1",
  balance_status: "已读取",
  checked_at: null,
};
const usage = [
  {
    upstream_id: host.upstream_id,
    request_count: 17,
    request_window_days: 30,
    account_request_usage: [],
  },
];
function renderPage(readUsage: () => Promise<Response>, hosts = [host]): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path === "/api/upstreams") return Response.json({ hosts });
    if (path === "/api/upstreams/usage") return readUsage();
    if (path === "/api/config") return Response.json({ mode: "监控模式" });
    return Response.json({ items: [] });
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  render(
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}>
        <UpstreamsPage />
      </RouterContextProvider>
    </QueryClientProvider>,
  );
}
it("用量尚未返回时先显示列表与操作，返回后补齐请求数", async () => {
  let resolveUsage!: (response: Response) => void;
  const pending = new Promise<Response>((resolve) => {
    resolveUsage = resolve;
  });
  renderPage(() => pending);
  expect(await screen.findByRole("checkbox", { name: "选择上游 测试上游" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "编辑上游" })).toBeEnabled();
  expect(screen.getByRole("status", { name: "正在读取用量" })).toBeVisible();
  expect(screen.queryByText("0 次")).not.toBeInTheDocument();
  await act(async () => resolveUsage(Response.json(usage)));
  expect(await screen.findByText("17 次")).toBeVisible();
});

it("列表为空时展示空状态且不发起用量请求", async () => {
  const readUsage = vi.fn(async () => Response.json([]));
  renderPage(readUsage, []);
  expect(await screen.findByText("当前业务库没有上游 Host")).toBeVisible();
  expect(readUsage).not.toHaveBeenCalled();
});

it("统计明确返回零请求时显示零且允许查看空账号明细", async () => {
  renderPage(async () => Response.json([{ ...usage[0], request_count: 0 }]));
  expect(await screen.findByText("0 次")).toBeVisible();
  expect(screen.getByRole("button", { name: "查看测试上游的账号使用率" })).toBeEnabled();
});
it("用量失败时保留列表，重试成功后显示统计", async () => {
  let failed = true;
  renderPage(async () =>
    failed ? Response.json({ error: "用量读取失败" }, { status: 500 }) : Response.json(usage),
  );
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "编辑上游" })).toBeEnabled();
  failed = false;
  await userEvent.setup().click(retry);
  expect(await screen.findByText("17 次")).toBeVisible();
});
it("后台用量刷新期间及失败后保留已有请求数和列表选择", async () => {
  let readUsage = async (): Promise<Response> => Response.json(usage);
  renderPage(() => readUsage());
  await screen.findByText("17 次");
  const checkbox = screen.getByRole("checkbox", { name: "选择上游 测试上游" });
  await userEvent.setup().click(checkbox);
  let resolveUsage!: (response: Response) => void;
  readUsage = () =>
    new Promise<Response>((resolve) => {
      resolveUsage = resolve;
    });
  let refresh!: Promise<void>;
  act(() => {
    refresh = client.invalidateQueries({ queryKey: ["upstreams", "usage"] });
  });
  await waitFor(() => expect(client.isFetching({ queryKey: ["upstreams", "usage"] })).toBe(1));
  expect(screen.getByText("17 次")).toBeVisible();
  expect(checkbox).toBeChecked();
  await act(async () => {
    resolveUsage(Response.json({ error: "用量读取失败" }, { status: 500 }));
    await refresh;
  });
  expect(screen.getByText("17 次")).toBeVisible();
  expect(checkbox).toBeChecked();
});
