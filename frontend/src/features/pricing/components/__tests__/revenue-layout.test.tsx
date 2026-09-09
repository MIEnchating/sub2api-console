import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";

import type { RevenueReport } from "@/api";
import { RevenueAnalysisPage } from "../revenue-analysis-page";
import { revenueReport, revenueTask } from "./revenue-fixtures";

function renderReport(report: RevenueReport | null = revenueReport): void {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["pricing-revenue-latest"], report ? revenueTask(report) : null);
  render(
    <QueryClientProvider client={client}>
      <RevenueAnalysisPage />
    </QueryClientProvider>,
  );
}

it("已有报告时分类独立于内容滚动区，方向键和首尾键同步切换视图与焦点", async () => {
  const user = userEvent.setup();
  renderReport();
  const tabs = screen.getByRole("tablist", { name: "收益分析视图" });
  expect(tabs.closest('[data-slot="page-content"]')).toBeNull();
  const details = screen.getByRole("tab", { name: "账号明细" });
  details.focus();
  await user.keyboard("{ArrowRight}");
  expect(screen.getByRole("tab", { name: "金额统计" })).toHaveFocus();
  expect(screen.getByRole("tabpanel", { name: "金额统计" })).toBeVisible();
  expect(details).toHaveAttribute("aria-selected", "false");
  await user.keyboard("{End}");
  expect(screen.getByRole("tab", { name: "上游读取问题" })).toHaveFocus();
  await user.keyboard("{Home}");
  expect(details).toHaveFocus();
  expect(details).toHaveAttribute("aria-selected", "true");
});

it.each([
  ["账号明细", "暂无账号核算明细"],
  ["金额统计", "暂无金额统计"],
  ["上游读取问题", "没有上游读取问题"],
])("%s没有数据时显示明确的空状态", async (tab, message) => {
  renderReport();
  await userEvent.setup().click(screen.getByRole("tab", { name: tab }));
  expect(screen.getByRole("cell", { name: message })).toBeVisible();
});

it("上游地址和失败原因超长时完整换行展示", async () => {
  const host = "very-long-upstream-address.example.test";
  const reason = "请求上游超时，请检查连接后重试。".repeat(10);
  renderReport({ ...revenueReport, issues: [{ host, reason }] });
  await userEvent.setup().click(screen.getByRole("tab", { name: "上游读取问题" }));
  expect(screen.getByRole("cell", { name: host })).toHaveClass(
    "whitespace-normal",
    "wrap-anywhere",
  );
  expect(screen.getByRole("cell", { name: reason })).toHaveClass(
    "whitespace-normal",
    "wrap-anywhere",
  );
});

it("尚无历史报告时居中展示提示且不显示空分类导航", () => {
  renderReport(null);
  expect(screen.getByText("尚未生成核算结果")).toHaveClass(
    "flex-1",
    "items-center",
    "justify-center",
  );
  expect(screen.queryByRole("tablist", { name: "收益分析视图" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "开始分析" })).toBeEnabled();
});
