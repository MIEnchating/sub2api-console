import { expect, test, type Page } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";
import { revenueTask } from "../../src/features/pricing/components/__tests__/revenue-fixtures";

async function isolate(page: Page, theme: string): Promise<void> {
  await page.addInitScript((value) => localStorage.setItem("sub2api-console-theme", value), theme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "收益交互测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": { items: [] },
      "/api/pricing/revenue/latest": revenueTask(),
      "/api/pricing/backups": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
}

test.beforeEach(async ({ page }, testInfo) => {
  if (testInfo.project.name === "mobile-dark")
    await page.setViewportSize({ width: 320, height: 568 });
  await isolate(page, testInfo.project.use.colorScheme ?? "light");
});

test("历史报告晚到保留键盘所选日期，提交核算使用该日期", async ({ page }, testInfo) => {
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/pricing/revenue/latest", async (route) => {
    await held;
    await route.fulfill({ json: revenueTask() });
  });
  await page.goto("/revenue-analysis");
  await expect(page.getByRole("status", { name: "正在读取最近一次分析" })).toBeVisible();
  const day = page.getByRole("spinbutton", { name: /^日,/ });
  await day.focus();
  await day.press("ArrowDown");
  await day.press("ArrowDown");
  await day.press("Tab");
  const year = await page.getByRole("spinbutton", { name: /^年,/ }).getAttribute("aria-valuenow");
  const month = await page.getByRole("spinbutton", { name: /^月,/ }).getAttribute("aria-valuenow");
  const chosenDay = await day.getAttribute("aria-valuenow");
  const chosenDate = `${year}-${month!.padStart(2, "0")}-${chosenDay!.padStart(2, "0")}`;
  release();
  await expect(page.getByRole("tabpanel", { name: "账号明细" })).toBeVisible();
  await expect(day).toHaveAttribute("aria-valuenow", chosenDay!);
  const request = page.waitForRequest(
    (item) => new URL(item.url()).pathname === "/api/pricing/revenue" && item.method() === "POST",
  );
  await page.getByRole("button", { name: "开始分析", exact: true }).click();
  expect((await request).postDataJSON()).toEqual({ date: chosenDate });
  await page.screenshot({ path: testInfo.outputPath("selected-date.png") });
});

test("离页取消正在读取的历史报告", async ({ page }) => {
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/pricing/revenue/latest", async (route) => {
    await held;
    await route.fulfill({ json: revenueTask() });
  });
  try {
    await page.goto("/revenue-analysis");
    await expect(page.getByRole("status", { name: "正在读取最近一次分析" })).toBeVisible();
    const link = page.getByRole("link", { name: "价格管理", exact: true });
    if (!(await link.isVisible())) await page.getByRole("button", { name: "切换侧边栏" }).click();
    const cancelled = page.waitForEvent("requestfailed", {
      predicate: (request) => new URL(request.url()).pathname === "/api/pricing/revenue/latest",
    });
    await link.click();
    expect((await cancelled).failure()?.errorText).toContain("ERR_ABORTED");
    await expect(page).toHaveURL(/\/pricing$/);
  } finally {
    release();
  }
});

test("无效读取问题显示悬浮错误与重试，键盘重试恢复报告", async ({ page }, testInfo) => {
  let valid = false;
  await page.route("**/api/pricing/revenue/latest", (route) =>
    route.fulfill({
      json: valid
        ? revenueTask()
        : { ...revenueTask(), result: { ...revenueTask().result, issues: [null] } },
    }),
  );
  await page.goto("/revenue-analysis");
  const retry = page.getByRole("button", { name: "重新读取" });
  await expect(retry).toBeVisible({ timeout: 15_000 });
  await expect(
    page.getByText("最近收益报告格式无效，请重新读取或重新分析", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("尚未生成核算结果")).toHaveCount(0);
  valid = true;
  await retry.focus();
  await retry.press("Enter");
  await expect(page.getByRole("tabpanel", { name: "账号明细" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: testInfo.outputPath("retry-ready.png") });
});
