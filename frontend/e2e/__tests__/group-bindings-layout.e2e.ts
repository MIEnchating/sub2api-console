import { expect, test } from "@playwright/test";

import { pageFixtures, workspace } from "./fixtures/page-shell";

const groups = Array.from({ length: 25 }, (_, index) => ({
  id: `group-${index + 1}`,
  name: `测试分组 ${index + 1}`,
  ratio: "1",
}));

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
      "/api/auth/session": { authenticated: true, username: "间距测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": { items: [] },
      "/api/newapi": {
        ...workspace,
        bindings: [
          {
            platform_id: "layout",
            newapi_group_id: "group-1",
            newapi_group_name: "测试分组 1",
            sub2api_group_id: "6",
            sync_ratio: true,
          },
        ],
      },
      "/api/newapi/platforms/layout/refresh": {
        groups,
        models: [],
        unset_models: [],
        references: [],
        tool_prices: [],
        differences: [],
        upstream_prices: [],
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

test("分组工具栏到表格只保留常规间距，错误修正后恢复且分页可达", async ({ page }) => {
  await page.goto("/newapi/groups");
  const toolbar = page.locator('[data-slot="table-filter-toolbar"]');
  const panel = page.locator("[data-table-panel]");
  const ratio = page.getByRole("textbox", {
    name: "测试分组 1 的 Sub2API 管理平台倍率",
    exact: true,
  });
  await expect(ratio).toHaveValue("1");
  const toolbarBox = (await toolbar.boundingBox())!;
  const initialPanelBox = (await panel.boundingBox())!;
  const gap = initialPanelBox.y - toolbarBox.y - toolbarBox.height;
  expect(gap).toBeGreaterThanOrEqual(0);
  expect(gap).toBeLessThanOrEqual(12);
  await expect(page.locator('[data-slot="field-error"]')).toHaveCount(0);
  await page.screenshot({ path: test.info().outputPath("group-bindings-compact.png") });

  await ratio.fill("0");
  await expect(page.getByRole("alert")).toHaveText("Sub2API 管理平台倍率必须大于 0");
  await expect(ratio).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByRole("button", { name: "保存绑定与倍率" })).toBeDisabled();
  const errorBox = (await page.getByRole("alert").boundingBox())!;
  expect(errorBox.y).toBeGreaterThanOrEqual(toolbarBox.y + toolbarBox.height);
  expect(errorBox.y + errorBox.height).toBeLessThanOrEqual((await panel.boundingBox())!.y);
  await ratio.fill("1");
  await expect(page.locator('[data-slot="field-error"]')).toHaveCount(0);
  expect((await panel.boundingBox())!.y).toBe(initialPanelBox.y);
  await expect(page.getByRole("button", { name: "保存绑定与倍率" })).toBeEnabled();

  const next = page.getByRole("button", { name: "转到下一页" });
  await next.scrollIntoViewIfNeeded();
  await expect(next).toBeInViewport();
  await next.click();
  await expect(page.getByText("测试分组 25", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

test("筛选为单项或空结果时工具栏下方不出现空提示行", async ({ page }) => {
  await page.goto("/newapi/groups");
  const search = page.getByRole("textbox", { name: "搜索 New API 分组或 ID" });
  await expect(page.getByRole("table")).toBeVisible();
  await search.fill("group-25");
  await expect(page.getByRole("row")).toHaveCount(2);
  await expect(page.getByText("测试分组 25", { exact: true })).toBeVisible();
  await expect(page.locator('[data-slot="field-error"]')).toHaveCount(0);
  await search.fill("不存在的分组");
  await expect(page.getByText("没有匹配的 New API 分组")).toBeVisible();
  await expect(page.locator('[data-slot="field-error"]')).toHaveCount(0);
});
