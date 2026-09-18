import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";
import { policy } from "./fixtures/settings";

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "移动端布局回归" },
      "/api/dictionaries": { items: [] },
      "/api/policy": policy,
      "/api/accounts": [],
      "/api/groups": [],
      "/api/tasks": [],
      "/api/inspection/automation": {
        enabled: false,
        interval_seconds: 15,
        running: false,
        traffic_collection: { enabled: false },
        monitoring_enabled: false,
        monitoring_configured: true,
        queue: [],
        heartbeat_history: [],
      },
      "/api/system/metrics": {
        cpu: { usage_percent: 20, logical_cores: 4 },
        memory: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
        disk: { usage_percent: 40, used_bytes: 1024, total_bytes: 4096 },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

for (const [path, title] of [
  ["/accounts", "账号管理"],
  ["/groups", "分组管理"],
  ["/upstreams", "上游管理"],
  ["/alerts", "告警通知"],
  ["/logs?kind=event", "日志中心"],
  ["/system-info", "系统信息"],
  ["/newapi/groups", "分组绑定"],
  ["/newapi/differences", "价格比对"],
]) {
  test(`320px ${title}筛选换行后列表保留可读高度，内容不会横向溢出`, async ({ page }) => {
    await page.goto(path);
    await expect(page.getByRole("heading", { name: title, exact: true })).toBeVisible();
    await expect(page.locator('[data-slot="skeleton"]')).toHaveCount(0);
    const container =
      path === "/alerts"
        ? page.getByTestId("alert-list-scroll-area")
        : page.locator('[data-slot="table-container"]').first();
    await expect(container).toBeVisible();
    await expect
      .poll(() => container.evaluate((element) => element.clientHeight))
      .toBeGreaterThanOrEqual(160);
    const content = page.locator('[data-slot="page-content"]');
    expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await container.scrollIntoViewIfNeeded();
    await page.screenshot({ path: test.info().outputPath("mobile-workspace.png") });
  });
}

for (const [path, message] of [
  ["/upstreams", "当前业务库没有上游 Host"],
  ["/auto-inspection", "暂无心跳记录，启用自动巡检后会记录每次调度检查"],
]) {
  test(`320px ${path}空状态说明在可视宽度内完整换行`, async ({ page }) => {
    await page.goto(path);
    const empty = page.getByText(message, { exact: true });
    await empty.scrollIntoViewIfNeeded();
    await expect(empty).toBeInViewport({ ratio: 1 });
    const bounds = await empty.boundingBox();
    expect(bounds!.width).toBeLessThanOrEqual(296);
    expect(await empty.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    const table = empty.locator('xpath=ancestor::div[@data-slot="table-container"]');
    expect(await table.evaluate((element) => element.scrollLeft)).toBe(0);
  });
}
