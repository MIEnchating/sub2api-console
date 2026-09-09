import { expect, test } from "@playwright/test";

import {
  revenueReport,
  revenueRow,
  revenueTask,
} from "../../src/features/pricing/components/__tests__/revenue-fixtures";

const report = {
  ...revenueReport,
  rows: Array.from({ length: 24 }, (_, index) => ({
    ...revenueRow,
    account_id: String(index + 1),
    account_name: `核算账号 ${index + 1}`,
  })),
  issues: [
    {
      host: "very-long-upstream-address.example.test",
      reason: "上游暂时不可用，请检查连接后重新核算。".repeat(12),
    },
  ],
};

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "收益布局测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/pricing/revenue/latest": revenueTask(report),
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

test("低高度窗口中分类与日期始终可见，表格横向滚动和分页均可操作", async ({ page, viewport }) => {
  await page.setViewportSize({ width: viewport!.width, height: 480 });
  await page.goto("/revenue-analysis");
  const tabs = page.getByRole("tablist", { name: "收益分析视图" });
  await expect(page.getByText("核算账号 1", { exact: true })).toBeVisible();
  const content = page.locator('[data-slot="page-content"]');
  const table = page.locator('[data-slot="table-container"]');
  await table.evaluate((element) => element.scrollTo(element.scrollWidth, element.scrollHeight));
  const next = page.getByRole("button", { name: "转到下一页", exact: true });
  await next.scrollIntoViewIfNeeded();
  await expect(next).toBeInViewport({ ratio: 1 });
  await expect(tabs).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("button", { name: "开始分析", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await next.click();
  await table.evaluate((element) => element.scrollTo(0, 0));
  await expect(page.getByText("核算账号 21", { exact: true })).toBeVisible();
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  const details = page.getByRole("tab", { name: "账号明细" });
  await details.focus();
  await page.keyboard.press("End");
  await expect(page.getByRole("tab", { name: "上游读取问题" })).toBeFocused();
  await expect(page.getByRole("columnheader", { name: "Host", exact: true })).toBeInViewport({
    ratio: 1,
  });
  const reason = page.getByRole("cell", { name: report.issues[0].reason });
  await expect(reason).toBeVisible();
  expect(await reason.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("revenue-issues.png") });
});

test("报告为空时各视图显示空状态且窄屏分类不被裁切", async ({ page }) => {
  await page.route("**/api/pricing/revenue/latest", (route) =>
    route.fulfill({ json: revenueTask() }),
  );
  await page.goto("/revenue-analysis");
  for (const [label, empty] of [
    ["账号明细", "暂无账号核算明细"],
    ["金额统计", "暂无金额统计"],
    ["上游读取问题", "没有上游读取问题"],
  ]) {
    const tab = page.getByRole("tab", { name: label });
    await expect(tab).toBeInViewport({ ratio: 1 });
    await tab.click();
    await expect(page.getByText(empty, { exact: true })).toBeInViewport({ ratio: 1 });
  }
});

test("启动后进度居中并禁用重复操作，任务失败后展示原因并允许重试", async ({ page }) => {
  let finished = false;
  const running = {
    ...revenueTask(),
    status: "running",
    progress: 45,
    message: "正在核算",
    result: {},
  };
  await page.route("**/api/pricing/revenue", (route) => route.fulfill({ json: running }));
  await page.route("**/api/tasks/revenue-layout", (route) =>
    route.fulfill({
      json: finished
        ? { ...running, status: "failed", message: "上游读取超时，请稍后重新分析" }
        : running,
    }),
  );
  await page.goto("/revenue-analysis");
  await page.getByRole("button", { name: "开始分析", exact: true }).click();
  await expect(page.getByRole("button", { name: "核算中", exact: true })).toBeDisabled();
  await expect(page.getByText("45%", { exact: true })).toBeVisible();
  await expect(page.getByRole("tablist", { name: "收益分析视图" })).toHaveCount(0);
  finished = true;
  await expect(
    page.getByRole("alert").filter({ hasText: "上游读取超时，请稍后重新分析" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "开始分析", exact: true })).toBeEnabled();
});
