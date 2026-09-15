import { expect, test } from "@playwright/test";

import type { NewAPIRemoteSnapshot } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("大量上游失败时详情在视口内滚动且关闭后仍能比对可用上游", async ({ page, colorScheme }) => {
  const warning = [
    `${"long-host-".repeat(20)}.test：/api/v1/model-plaza：上游鉴权失败（HTTP 401），请在鉴权恢复中恢复该上游登录态后刷新`,
    ...Array.from(
      { length: 25 },
      (_, index) =>
        `unavailable-${index}.test：/api/v1/model-plaza：上游价格接口未开放或不存在（HTTP 404），请确认站点版本支持并启用模型广场`,
    ),
  ].join("\n");
  const snapshot: NewAPIRemoteSnapshot = {
    groups: [],
    models: [{ model: "test-model", input_ratio: "0.5", completion_ratio: "4" }],
    unset_models: [],
    references: [],
    tool_prices: [],
    differences: [],
    fetched_at: "2026-09-14T08:00:00Z",
    upstream_price_warning: warning,
    upstream_prices: [
      {
        host: "available.test",
        name: "可用上游",
        upstream_type: "newapi",
        models: [{ model: "test-model", input_ratio: "0.5", completion_ratio: "4" }],
      },
    ],
  };
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价格测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/refresh": snapshot,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });

  await page.goto("/newapi/differences");
  const details = page.getByRole("region", { name: "错误详情" });
  const notification = page.locator('[data-sonner-toast][data-type="error"]');
  await expect(page.getByText("上游价格读取失败", { exact: true })).toBeVisible();
  await expect(details).toHaveText(warning);
  await expect(notification).toBeInViewport({ ratio: 1 });
  const viewport = page.viewportSize()!;
  expect((await notification.boundingBox())!.height).toBeLessThan(viewport.height / 2);
  expect(await details.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await details.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  );
  await details.focus();
  await details.press("End");
  await expect
    .poll(() =>
      details.evaluate(
        (element) => element.scrollTop + element.clientHeight >= element.scrollHeight,
      ),
    )
    .toBe(true);
  await expect(details).toBeFocused();
  await expect(page.getByRole("button", { name: "关闭通知" })).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: test.info().outputPath("upstream-price-errors.png") });

  await page.getByRole("button", { name: "关闭通知" }).click();
  await expect(details).toHaveCount(0);
  await page.getByRole("combobox", { name: "比对上游" }).click();
  await page.getByRole("option", { name: /available\.test · New API/ }).click();
  await page.getByRole("button", { name: "比对 test-model" }).click();
  await expect(page.getByRole("dialog", { name: "test-model 价格比对" })).toBeVisible();
  await expect(page.getByText("价格一致", { exact: true })).toBeVisible();
});
