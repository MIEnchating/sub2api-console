import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ImportPanel } from "../components/import-panel";
import { importModelKey, templateKeys } from "../constants";
import { importPreviewFixture } from "./import-preview-fixture";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function mount(fetcher: typeof fetch): void {
  vi.stubGlobal("fetch", fetcher);
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  client.setQueryData(importModelKey, { model: "gpt-6-astra", revision: 0 });
  client.setQueryData(templateKeys.library, { revision: 0, preferred_id: "", items: [] });
  render(
    <QueryClientProvider client={client}>
      <ImportPanel onStarted={() => undefined} />
    </QueryClientProvider>,
  );
}
const proxy = { id: "3", name: "固定出口", protocol: "http", available: true };

it("同步后选择平台代理只提交稳定 ID，预览展示统一代理", async () => {
  let body: Record<string, unknown> | undefined;
  mount(
    vi.fn(async (input, init) => {
      if (String(input).endsWith("/proxies")) return Response.json([proxy]);
      body = JSON.parse(String(init?.body));
      return Response.json({ ...importPreviewFixture(1), proxy });
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "同步平台代理" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "同步平台代理" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "统一平台代理" }));
  await user.click(await screen.findByRole("option", { name: /固定出口/ }));
  expect(screen.queryByRole("checkbox", { name: "使用登录 / 检测代理" })).not.toBeInTheDocument();
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_fixture");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByText(/登录、检测与账号调用代理：固定出口/)).toBeVisible();
  expect(body).toMatchObject({ platform_proxy_id: "3", proxy_enabled: false, proxy_url: "" });
});

it("首次读取代理期间禁止提交，空列表提供配置提示", async () => {
  let resolve: (response: Response) => void = () => undefined;
  mount(
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "同步平台代理" }));
  expect(screen.getByText("正在读取平台代理")).toBeVisible();
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeDisabled();
  resolve(Response.json([]));
  expect(await screen.findByText(/平台暂无代理/)).toBeVisible();
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeEnabled();
});

it("代理读取失败保留重试入口，重试后不可用代理不能选择", async () => {
  let failed = true;
  mount(
    vi.fn(async () =>
      failed
        ? Response.json({ message: "读取失败" }, { status: 502 })
        : Response.json([{ ...proxy, available: false, reason: "代理未启用" }]),
    ),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "同步平台代理" }));
  await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeDisabled();
  failed = false;
  await user.click(screen.getByRole("button", { name: "重新读取" }));
  await screen.findByText(/固定出口.*代理未启用/);
  await user.click(screen.getByRole("combobox", { name: "统一平台代理" }));
  expect(screen.queryByRole("option", { name: /固定出口/ })).not.toBeInTheDocument();
  expect(screen.getByRole("option", { name: "沿用模板代理" })).toBeVisible();
});

it("后台同步保留已选代理，代理删除后禁止提交并允许键盘清除", async () => {
  let proxies = [proxy];
  mount(vi.fn(async () => Response.json(proxies)));
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "同步平台代理" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "同步平台代理" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "统一平台代理" }));
  await user.click(await screen.findByRole("option", { name: /固定出口/ }));
  proxies = [];
  expect(screen.getByRole("combobox", { name: "统一平台代理" })).toHaveTextContent("固定出口");
  await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "同步平台代理" }));
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "统一平台代理" })).toHaveTextContent("已选代理 #3"),
  );
  await screen.findByText(/已选代理不可用/);
  expect(screen.getByRole("button", { name: "解析并预览" })).toBeDisabled();
  screen.getByRole("combobox", { name: "统一平台代理" }).focus();
  await user.keyboard("{Enter}");
  await user.click(screen.getByRole("option", { name: "沿用模板代理" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "解析并预览" })).toBeEnabled());
});
