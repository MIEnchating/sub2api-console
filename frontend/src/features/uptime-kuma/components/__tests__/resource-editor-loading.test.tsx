import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { KumaResourcesPage } from "../resources-page";
import { kumaConfigKey, kumaResourcesKey } from "../../constants";
import { config } from "./fixtures";
import { statusOptions, statusPage } from "./status-page-fixtures";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function renderPage(): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(kumaConfigKey, config);
  client.setQueryData([...kumaResourcesKey, "status-pages", config.revision], statusOptions);
  render(
    <QueryClientProvider client={client}>
      <KumaResourcesPage kind="status-pages" />
    </QueryClientProvider>,
  );
}

it("点击编辑立即打开弹窗，轻量加载提示仅在弹窗内，读取后回显表单", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  renderPage();
  const table = screen.getByRole("table", { name: "状态页管理" });
  fireEvent.click(screen.getByRole("button", { name: "编辑" }));
  const dialog = screen.getByRole("dialog", { name: "编辑状态页管理" });
  expect(within(dialog).getByRole("status", { name: "正在读取编辑配置…" })).toBeVisible();
  expect(within(dialog).getByText("正在读取编辑配置…")).toBeVisible();
  expect(dialog.querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(table).toBeInTheDocument();
  expect(within(dialog).getByRole("button", { name: "保存" })).toBeDisabled();
  expect(within(dialog).getByRole("button", { name: "取消" })).toBeEnabled();
  await act(async () => {
    resolve(Response.json(statusPage));
  });
  expect(await screen.findByLabelText("状态页标题")).toHaveValue("服务状态");
  expect(screen.queryByRole("status", { name: "正在读取编辑配置…" })).not.toBeInTheDocument();
});

it("读取中取消编辑，迟到的详情不会重新打开弹窗", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  renderPage();
  fireEvent.click(screen.getByRole("button", { name: "编辑" }));
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  await act(async () => {
    resolve(Response.json(statusPage));
  });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("详情读取失败保留弹窗并禁用保存，重新读取成功后显示表单", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json({ detail: "连接失败" }, { status: 502 }))
      .mockResolvedValueOnce(Response.json(statusPage)),
  );
  renderPage();
  fireEvent.click(screen.getByRole("button", { name: "编辑" }));
  fireEvent.click(await screen.findByRole("button", { name: "重新读取" }));
  expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  expect(await screen.findByLabelText("状态页标题")).toHaveValue("服务状态");
});
