import { expect, test } from "@playwright/test";
import type { UsageRecord } from "../../src/api";

const host = `${"long-upstream-host-".repeat(12)}.example.test`;
const record: UsageRecord = {
  id: 1,
  request_id: "req-layout",
  account_id: null,
  account_name: null,
  group_name: null,
  is_error: false,
  error_reason: null,
  first_token_ms: null,
  duration_ms: "100",
  summary: "http request completed",
  observed_at: "2026-09-09T00:00:00Z",
  source: "system-log",
  payload: { host },
};

test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/ops/system-logs": { items: [record], total: 1, page: 1, page_size: 20 },
    };
    if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

test("查询包含超长 Host 的日志时完整换行，结果和分页不超出工作区", async ({ page }) => {
  await page.goto("/trace");
  await page.getByRole("textbox", { name: "request_id" }).fill("req-layout");
  await page.getByRole("button", { name: "查询", exact: true }).click();
  const hostLabel = page.getByText(`Host：${host}`, { exact: true });
  await expect(hostLabel).toBeVisible();
  expect(await hostLabel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.getByRole("button", { name: "转到下一页" }).scrollIntoViewIfNeeded();
  await expect(page.getByRole("button", { name: "转到下一页" })).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("heading", { name: "请求追踪", exact: true })).toBeInViewport({
    ratio: 1,
  });
});

test("查询失败时展示原因和重试提示，再次查询成功后才展示空结果", async ({ page }) => {
  let failed = true;
  await page.route("**/api/ops/system-logs?**", (route) => {
    if (failed) return route.fulfill({ status: 503, json: { detail: "日志服务暂不可用" } });
    return route.fulfill({ json: { items: [], total: 0, page: 1, page_size: 20 } });
  });
  await page.goto("/trace");
  await page.getByRole("textbox", { name: "request_id" }).fill("req-layout");
  await page.getByRole("button", { name: "查询", exact: true }).click();
  const content = page.locator('[data-slot="page-content"]');
  await expect(page.locator("[data-sonner-toast]")).toContainText("日志服务暂不可用");
  await expect(content.getByRole("alert")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "查询", exact: true })).toBeEnabled();
  await expect(page.getByText("没有匹配的系统日志", { exact: true })).toHaveCount(0);
  failed = false;
  await page.getByRole("button", { name: "查询", exact: true }).click();
  await expect(page.getByText("没有匹配的系统日志", { exact: true })).toBeVisible();
  await expect(content.getByRole("alert")).toHaveCount(0);
});
