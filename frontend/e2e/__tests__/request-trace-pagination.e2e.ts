import { expect, test } from "@playwright/test";

import type { UsageRecord } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

const record: UsageRecord = {
  id: 1,
  request_id: "request-pagination",
  account_id: null,
  account_name: null,
  group_name: null,
  is_error: false,
  error_reason: null,
  first_token_ms: null,
  duration_ms: null,
  summary: "HTTP request completed",
  observed_at: null,
  source: "system-log",
  payload: {},
};

for (const remaining of [1, 0]) {
  test(`请求追踪第二页变空且剩余 ${remaining} 条时分页可见并能返回首页`, async ({
    page,
  }, testInfo) => {
    if (testInfo.project.name === "mobile-dark") {
      await page.setViewportSize({ width: 320, height: 568 });
    }
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme),
      testInfo.project.use.colorScheme ?? "light",
    );
    let shrunk = false;
    await page.route("**/api/**", async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname === "/api/ops/system-logs") {
        const currentPage = Number(url.searchParams.get("page"));
        if (currentPage === 2) shrunk = true;
        await route.fulfill({
          json: {
            page: currentPage,
            page_size: Number(url.searchParams.get("page_size")),
            total: shrunk ? remaining : 21,
            items: currentPage === 1 && (!shrunk || remaining > 0) ? [record] : [],
          },
        });
        return;
      }
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "请求追踪测试" },
        "/api/dictionaries": { items: [] },
      };
      if (url.pathname.endsWith("/events")) {
        await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
      } else if (url.pathname in fixtures) {
        await route.fulfill({ json: fixtures[url.pathname] });
      } else {
        await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      }
    });
    await page.goto("/trace");
    await page.getByRole("textbox", { name: "request_id" }).fill("request-pagination");
    await page.getByRole("button", { name: "查询", exact: true }).click();
    await expect(page.getByText("request-pagination", { exact: true })).toBeVisible();
    const pagination = page.getByRole("navigation", { name: "表格分页" });
    await pagination.getByRole("button", { name: "转到下一页" }).click();
    await expect(
      page.getByText(remaining ? "当前页没有系统日志" : "没有匹配的系统日志"),
    ).toBeVisible();
    await expect(pagination).toBeInViewport({ ratio: 1 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("empty-page.png") });
    const previous = pagination.getByRole("button", { name: "转到上一页" });
    await previous.focus();
    await previous.press("Enter");
    await expect(previous).toBeDisabled();
    await page.getByRole("button", { name: "查询", exact: true }).click();
    if (remaining) {
      await expect(page.getByText("request-pagination", { exact: true })).toBeVisible();
    } else {
      await expect(page.getByText("没有匹配的系统日志")).toBeVisible();
    }
    const pageSize = pagination.getByRole("combobox", { name: "每页行数" });
    await pageSize.focus();
    await pageSize.press("Enter");
    await expect(pageSize).toHaveAttribute("aria-expanded", "true");
    const resized = page.waitForResponse((response) => {
      const url = new URL(response.url());
      return url.pathname === "/api/ops/system-logs" && url.searchParams.get("page_size") === "10";
    });
    await page.getByRole("option", { name: "10", exact: true }).click();
    expect(await (await resized).json()).toMatchObject({
      page: 1,
      page_size: 10,
      total: remaining,
    });
    await expect(pageSize).toContainText("10");
    await expect(previous).toBeDisabled();
    await pagination.scrollIntoViewIfNeeded();
    await expect(pagination).toBeInViewport({ ratio: 1 });
    await page.screenshot({ path: testInfo.outputPath("first-page.png") });
  });
}
