import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

const expression = `tier("${"long-context-".repeat(12)}", p * 3 + c * 15 + cr * 0.3 + cc * 3.75 + cc1h * 6)`;
const remote = {
  model: "claude-sonnet-4-6-thinking",
  input_price: "0.000003",
  output_price: "0.000015",
  cache_read_price: "0.0000003",
  cache_write_price: "0.00000375",
  cache_write_1h_price: "0.000006",
  model_ratio: "1.5",
  completion_ratio: "5",
  billing_expr: expression,
};

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价格差异测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/refresh": {
        groups: [],
        unset_models: [],
        references: [],
        tool_prices: [],
        differences: [],
        fetched_at: "2026-09-17T00:00:00Z",
        models: [
          {
            model: remote.model,
            input_ratio: "1.5",
            completion_ratio: "5",
            billing_mode: "tiered_expr",
            billing_expr: `${expression} + 100`,
          },
          {
            model: "reordered-model",
            input_ratio: "1.5",
            completion_ratio: "5",
            billing_mode: "tiered_expr",
            billing_expr: expression.replace("p * 3 + c * 15", "c * 15 + p * 3"),
          },
        ],
      },
      "/api/newapi/platforms/layout/management-model-prices": {
        models: [remote, { ...remote, model: "reordered-model" }],
        missing_models: [],
        stale: false,
      },
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

test("加法项换序显示一致，附加费用在差异弹窗中展示并允许长表达式换行", async ({ page }) => {
  await page.goto("/newapi/prices");
  await page.getByRole("button", { name: "比较模型价格" }).click();
  const reordered = page.getByRole("row", { name: /reordered-model/ });
  await expect(reordered.getByText("一致", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: `查看 ${remote.model} 价格差异` }).click();

  const dialog = page.getByRole("dialog", { name: remote.model });
  await expect(dialog.getByRole("table")).toHaveCSS("min-width", "704px");
  await expect(dialog.locator('[data-slot="table-container"]')).toHaveCSS("overflow-x", "auto");
  const expressionRow = dialog.getByRole("row", { name: /计费表达式/ });
  await expect(expressionRow.getByText("不一致", { exact: true })).toBeVisible();
  const configured = expressionRow.getByRole("cell", { name: `${expression} + 100`, exact: true });
  await expect(configured).toHaveCSS("white-space", "pre-wrap");
  await expect(configured).toHaveCSS("overflow-wrap", "anywhere");
  expect(await configured.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await expressionRow.getByText("不一致", { exact: true }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: test.info().outputPath("price-comparison-detail.png") });
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
});

test("批量同步预览包含长表达式时限制文本宽度并保留取消入口", async ({ page }) => {
  await page.goto("/newapi/prices");
  await page.getByRole("checkbox", { name: `选择模型 ${remote.model}`, exact: true }).click();
  await page.getByRole("button", { name: "批量同步（1）" }).click();

  const dialog = page.getByRole("dialog");
  const configured = dialog.getByText(`计费表达式：${expression} + 100`, { exact: true });
  await expect(configured).toBeVisible();
  await expect(configured).toHaveCSS("white-space", "pre-wrap");
  await expect(configured).toHaveCSS("overflow-wrap", "anywhere");
  await expect(configured).toHaveCSS("max-width", "384px");
  expect(await configured.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).not.toBeVisible();
});
