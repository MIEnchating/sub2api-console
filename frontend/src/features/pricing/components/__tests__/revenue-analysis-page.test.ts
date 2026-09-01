import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { Task } from "@/api";
import {
  defaultRevenueDate,
  RevenueCalculationProgress,
  RevenueAnalysisPage,
  revenueDateValue,
  revenueReportFromTask,
} from "../revenue-analysis-page";

describe("revenue analysis", () => {
  it("renders revenue analysis as a standalone page", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    const markup = renderToStaticMarkup(
      createElement(
        QueryClientProvider,
        { client: queryClient },
        createElement(RevenueAnalysisPage),
      ),
    );

    expect(markup).toContain("收益分析");
    expect(markup).toContain("开始分析");
    expect(markup).toContain('data-trigger="DatePicker"');
    expect(markup).toContain('aria-label="核算日期"');
    expect(markup).toContain('data-slot="page-actions"');
    expect(markup).toContain('data-testid="revenue-analysis-page"');
    expect(markup).not.toContain('role="dialog"');
    expect(markup).not.toContain("计算收入");
  });

  it("shows the latest successful report immediately on entry", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    queryClient.setQueryData(["pricing-revenue-latest"], {
      id: "latest-revenue",
      skill: "sub2api-billing-reconciliation",
      operation: "revenue-calculation",
      status: "succeeded",
      progress: 100,
      message: "done",
      result: {
        report_date: "2026-08-29",
        timezone: "Asia/Shanghai",
        tolerance: 2,
        rows: [],
        summaries: [],
        issues: [],
        comparable: 3,
        unavailable: 1,
        abnormal: 0,
        generated_at: "2026-08-30T00:00:00Z",
      },
      created_at: "2026-08-30T00:00:00Z",
      updated_at: "2026-08-30T00:00:01Z",
    } satisfies Task);
    const markup = renderToStaticMarkup(
      createElement(
        QueryClientProvider,
        { client: queryClient },
        createElement(RevenueAnalysisPage),
      ),
    );

    expect(markup).not.toContain("精确核对 3");
    expect(markup).not.toContain("计费异常 0");
    expect(markup).not.toContain("无法核对 1");
    expect(markup).toContain(">账号明细<");
    expect(markup).toContain(">上游读取问题<");
    expect(markup).not.toContain("账号明细 0");
    expect(markup).not.toContain("上游读取问题 0");
    expect(markup).not.toContain("尚未生成核算结果");
  });

  it("uses the previous local calendar day by default", () => {
    expect(defaultRevenueDate(new Date(2026, 7, 30, 1, 0, 0))).toBe("2026-08-29");
    expect(defaultRevenueDate(new Date(2026, 0, 1, 1, 0, 0))).toBe("2025-12-31");
    expect(revenueDateValue("2026-08-29")?.getDate()).toBe(29);
    expect(revenueDateValue("2026-02-30")).toBeUndefined();
  });

  it("accepts only a completed revenue task with the report contract", () => {
    const task: Task = {
      id: "revenue",
      skill: "sub2api-billing-reconciliation",
      operation: "revenue-calculation",
      status: "succeeded",
      progress: 100,
      message: "done",
      result: {
        report_date: "2026-08-29",
        timezone: "Asia/Shanghai",
        tolerance: 2,
        rows: [],
        summaries: [],
        issues: [],
        comparable: 0,
        unavailable: 0,
        abnormal: 0,
        generated_at: "2026-08-30T00:00:00Z",
      },
      created_at: "2026-08-30T00:00:00Z",
      updated_at: "2026-08-30T00:00:01Z",
    };

    expect(revenueReportFromTask(task)?.report_date).toBe("2026-08-29");
    expect(revenueReportFromTask({ ...task, status: "running" })).toBeNull();
    expect(revenueReportFromTask({ ...task, result: { report_date: "2026-08-29" } })).toBeNull();
  });

  it("centers the progress track and percentage in the page body while calculation is running", () => {
    const markup = renderToStaticMarkup(
      createElement(RevenueCalculationProgress, { progress: 42 }),
    );

    expect(markup).toContain('aria-label="收益分析进度"');
    expect(markup).toContain('aria-valuenow="42"');
    expect(markup).toContain("flex min-h-0 flex-1 items-center justify-center");
    expect(markup).toContain("w-full max-w-md");
    expect(markup).toContain("正在分析");
    expect(markup).toContain(">42%<");
  });
});
