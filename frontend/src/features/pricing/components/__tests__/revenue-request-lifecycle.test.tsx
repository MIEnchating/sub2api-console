import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { Task } from "@/api";
import { RevenueAnalysisPage } from "../revenue-analysis-page";
import { revenueReport, revenueRow, revenueTask } from "./revenue-fixtures";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client.clear();
  vi.unstubAllGlobals();
});

function renderPage(cached = false): ReturnType<typeof render> {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  if (cached) client.setQueryData(["pricing-revenue-latest"], revenueTask());
  return render(
    <QueryClientProvider client={client}>
      <RevenueAnalysisPage />
    </QueryClientProvider>,
  );
}

it("离开收益分析页取消历史读取，晚到响应不写入缓存", async () => {
  let signal: AbortSignal | null | undefined;
  let resolve!: (response: Response) => void;
  vi.stubGlobal("fetch", (_input: RequestInfo | URL, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((done) => {
      resolve = done;
    });
  });
  const page = renderPage();
  await waitFor(() => expect(resolve).toBeDefined());
  page.unmount();
  expect(signal?.aborted).toBe(true);
  await act(async () => resolve(Response.json(revenueTask())));
  expect(client.getQueryData(["pricing-revenue-latest"])).toBeUndefined();
});

it("历史报告晚到时保留用户已选的核算日期", async () => {
  const user = userEvent.setup();
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  renderPage();
  const day = screen.getByRole("spinbutton", { name: /^日,/ });
  await user.click(day);
  await user.keyboard("{ArrowDown}{ArrowDown}");
  await user.tab();
  const selected = day.getAttribute("aria-valuenow")!;
  await act(async () => resolve(Response.json(revenueTask())));
  await screen.findByRole("tablist", { name: "收益分析视图" });
  expect(day).toHaveAttribute("aria-valuenow", selected);
});

it("同一报告后台刷新保留已选日期和金额统计视图", async () => {
  const user = userEvent.setup();
  renderPage(true);
  await user.click(screen.getByRole("tab", { name: "金额统计" }));
  const day = screen.getByRole("spinbutton", { name: /^日,/ });
  await user.click(day);
  await user.keyboard("{ArrowDown}");
  await user.tab();
  const selected = day.getAttribute("aria-valuenow")!;
  await act(async () => {
    client.setQueryData(
      ["pricing-revenue-latest"],
      revenueTask({
        ...revenueReport,
        summaries: [
          {
            group: "刷新统计",
            accounts: 1,
            account_cost: "1.000000",
            actual_cost: "1.000000",
            upstream_raw_cost: "1.000000",
            upstream_cost: "1.000000",
            difference: "0.000000",
            revenue: "0.000000",
          },
        ],
      }),
    );
  });
  expect(await screen.findByRole("cell", { name: "刷新统计" })).toBeVisible();
  expect(day).toHaveAttribute("aria-valuenow", selected);
  expect(screen.getByRole("tabpanel", { name: "金额统计" })).toBeVisible();
});

it("新报告到达后切回账号明细但保留用户核算日期", async () => {
  const user = userEvent.setup();
  renderPage(true);
  await user.click(screen.getByRole("tab", { name: "金额统计" }));
  const day = screen.getByRole("spinbutton", { name: /^日,/ });
  await user.click(day);
  await user.keyboard("{ArrowDown}");
  await user.tab();
  const selected = day.getAttribute("aria-valuenow")!;
  await act(async () => {
    client.setQueryData(
      ["pricing-revenue-latest"],
      revenueTask({
        ...revenueReport,
        report_date: "2026-09-06",
        generated_at: "2026-09-10T00:00:00Z",
        rows: [revenueRow],
      }),
    );
  });
  expect(await screen.findByRole("cell", { name: /核算账号/ })).toBeVisible();
  expect(day).toHaveAttribute("aria-valuenow", selected);
  expect(screen.getByRole("tabpanel", { name: "账号明细" })).toBeVisible();
});

it("历史报告明细结构无效时显示重试而非无历史结果，重试后展示有效报告", async () => {
  const user = userEvent.setup();
  let report: Task = { ...revenueTask(), result: { ...revenueReport, issues: [null] } };
  vi.stubGlobal("fetch", () => Promise.resolve(Response.json(report)));
  renderPage();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByText("尚未生成核算结果")).not.toBeInTheDocument();
  report = revenueTask();
  await user.click(retry);
  expect(await screen.findByRole("tabpanel", { name: "账号明细" })).toBeVisible();
});
