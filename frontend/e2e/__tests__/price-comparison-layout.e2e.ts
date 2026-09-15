import { expect, test } from "@playwright/test";

import type { NewAPIRemoteSnapshot } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

const longModel =
  "claude-sonnet-4-20250514-extended-context-model-name-for-narrow-layout-regression";

const snapshot: NewAPIRemoteSnapshot = {
  groups: [],
  models: [
    { model: longModel, input_ratio: "1", completion_ratio: "2" },
    ...Array.from({ length: 24 }, (_, index) => ({
      model: `layout-model-${String(index + 1).padStart(2, "0")}`,
      input_ratio: "1",
      completion_ratio: "2",
    })),
  ],
  unset_models: [],
  references: [],
  tool_prices: [],
  differences: [],
  fetched_at: "2026-09-15T00:00:00Z",
  upstream_prices: [
    {
      host: "layout-upstream.example.test",
      name: "布局测试上游",
      upstream_type: "newapi",
      models: [{ model: longModel, input_ratio: "1", completion_ratio: "2" }],
    },
  ],
};

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价格布局测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/refresh": snapshot,
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("筛选栏和分页在桌面与窄屏保持可用", async ({ page }) => {
  await page.goto("/newapi/differences");

  await expect(page.getByRole("combobox", { name: "比对上游" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "搜索比对模型" })).toBeVisible();
  await expect(page.getByRole("button", { name: "批量比对" })).toBeVisible();

  const table = page.locator('[data-slot="table"]');
  await expect(table).toBeVisible();
  await expect(table.getByText(longModel, { exact: true })).toBeVisible();

  const pagination = page.getByRole("navigation", { name: "表格分页" });
  await expect(pagination).toBeVisible();
  await expect(pagination.getByRole("button", { name: "转到下一页" })).toBeEnabled();
  await pagination.getByRole("button", { name: "转到下一页" }).click();
  await expect(table.getByText("layout-model-24", { exact: true })).toBeVisible();
});

test("长模型名触发表格局部横向滚动且页面本身不溢出", async ({ page }) => {
  await page.setViewportSize({ width: 640, height: 800 });
  await page.goto("/newapi/differences");
  const container = page.locator('[data-slot="table-container"]');
  await expect(container).toHaveCSS("overflow-x", "auto");

  const overflow = await container.evaluate((element) => ({
    horizontal: element.scrollWidth > element.clientWidth,
    pageWidth: document.documentElement.scrollWidth,
    viewportWidth: window.innerWidth,
    tableMinWidth: getComputedStyle(element.querySelector("table")!).minWidth,
  }));
  if (overflow.viewportWidth < 640) expect(overflow.horizontal).toBe(true);
  expect(overflow.tableMinWidth).toBe("768px");
  expect(overflow.pageWidth).toBeLessThanOrEqual(overflow.viewportWidth);
});
