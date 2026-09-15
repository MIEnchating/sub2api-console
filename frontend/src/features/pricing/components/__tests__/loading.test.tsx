import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { RevenueAnalysisPage } from "../revenue-analysis-page";
import { revenueTask } from "./revenue-fixtures";
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

it("收益分析首次读取时显示骨架屏，不使用进度条占位", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RevenueAnalysisPage />
    </QueryClientProvider>,
  );
  const loading = screen.getByRole("status", { name: /正在读取/ });
  expect(loading).toHaveAttribute("aria-busy", "true");
  expect(loading.querySelector('[data-slot="skeleton"]')).not.toBeNull();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(
    screen.getByTestId("revenue-navigation-skeleton").closest('[data-slot="page-content"]'),
  ).toBeNull();
  expect(loading.querySelectorAll("thead th")).toHaveLength(12);
  expect(loading.querySelector("table")).toHaveClass("min-w-[100rem]", "table-fixed");
});

function renderRevenue(cached = false): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  if (cached) client.setQueryData(["pricing-revenue-latest"], revenueTask());
  render(
    <QueryClientProvider client={client}>
      <RevenueAnalysisPage />
    </QueryClientProvider>,
  );
}

it("历史报告读取失败时显示重试入口，成功重试确认无报告后才显示空结果", async () => {
  let fail = true;
  vi.stubGlobal("fetch", async () =>
    fail ? Response.json({ detail: "读取失败" }, { status: 503 }) : Response.json(null),
  );
  renderRevenue();
  await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByText("尚未生成核算结果")).not.toBeInTheDocument();
  fail = false;
  fireEvent.click(screen.getByRole("button", { name: "重新读取" }));
  expect(await screen.findByText("尚未生成核算结果")).toBeVisible();
});

it("创建收益任务尚未返回时只显示启动等待，后端返回真实零进度后才显示百分比", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal("fetch", (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST")
      return new Promise<Response>((done) => {
        resolve = done;
      });
    return new Promise<Response>(() => {});
  });
  renderRevenue(true);
  fireEvent.click(screen.getByRole("button", { name: "开始分析" }));
  await screen.findByRole("status", { name: "正在创建收益核算任务" });
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.queryByText("0%")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "核算中" })).toBeDisabled();
  await act(async () =>
    resolve(
      Response.json({
        ...revenueTask(),
        status: "queued",
        progress: 0,
        result: {},
        message: "等待核算",
      }),
    ),
  );
  const progress = await screen.findByRole("progressbar");
  expect(progress).toHaveAttribute("aria-valuenow", "0");
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
});

it("历史报告后台刷新失败时保留已选择视图与报告，不重新显示骨架", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ detail: "读取失败" }, { status: 503 }));
  renderRevenue(true);
  fireEvent.click(screen.getByRole("tab", { name: "金额统计" }));
  await act(async () => {
    await client.refetchQueries({ queryKey: ["pricing-revenue-latest"], exact: true });
  });
  await waitFor(() => expect(screen.getByRole("tabpanel", { name: "金额统计" })).toBeVisible());
  expect(screen.queryByRole("status", { name: "正在读取最近一次分析" })).not.toBeInTheDocument();
  expect(screen.queryByText("尚未生成核算结果")).not.toBeInTheDocument();
});
