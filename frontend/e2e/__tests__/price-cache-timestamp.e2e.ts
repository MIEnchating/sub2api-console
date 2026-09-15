import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

test("缓存更新时间在表格底部显示且表格滚动后仍可见", async ({ page, colorScheme }) => {
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
      "/api/newapi/platforms/layout/management-model-prices": {
        fetched_at: "2026-09-14T07:05:35Z",
        stale: false,
        models: Array.from({ length: 12 }, (_, i) => ({
          model: `model-${i}`,
          input_price: "0.000001",
          output_price: "0.000004",
          model_ratio: "0.5",
          completion_ratio: "4",
        })),
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/newapi/prices");
  await page.getByRole("tab", { name: "远程模型价格", exact: true }).click();
  const timestamp = page.getByRole("status", { name: "价格缓存更新时间" });
  await expect(timestamp).toBeInViewport({ ratio: 1 });
  const table = page.getByRole("tabpanel").locator('[data-slot="table-container"]');
  await table.evaluate((element) => element.scrollTo(element.scrollWidth, element.scrollHeight));
  await expect(timestamp).toBeInViewport({ ratio: 1 });
  const paginationBounds = await page.getByRole("navigation", { name: "表格分页" }).boundingBox();
  const bounds = await timestamp.boundingBox();
  expect(bounds!.y).toBeGreaterThanOrEqual(paginationBounds!.y + paginationBounds!.height);
  expect(await timestamp.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("price-cache-footer.png") });
});
