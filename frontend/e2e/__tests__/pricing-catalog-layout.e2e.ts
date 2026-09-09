import { expect, test } from "@playwright/test";

import { pricing } from "./fixtures/settings";

const catalog = {
  ...pricing,
  groups: Array.from({ length: 24 }, (_, index) => ({
    ...pricing.groups[0],
    id: String(index + 1),
    name: `价格分组 ${index + 1}`,
  })),
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
      "/api/auth/session": { authenticated: true, username: "价格布局测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/pricing": catalog,
      "/api/pricing/backups": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

test("低高度窗口滚动和分页后搜索栏仍可见，筛选从第一页展示结果", async ({ page, viewport }) => {
  await page.setViewportSize({ width: viewport!.width, height: 480 });
  await page.goto("/pricing");
  await expect(page.getByText("价格分组 1", { exact: true })).toBeVisible();
  const search = page.getByRole("textbox", { name: "搜索分组、ID 或平台" });
  const content = page.locator('[data-slot="page-content"]');
  const table = page
    .getByTestId("pricing-catalog-table-frame")
    .locator('[data-slot="table-container"]');
  await table.evaluate((element) => element.scrollTo(element.scrollWidth, element.scrollHeight));
  const next = page.getByRole("button", { name: "转到下一页", exact: true });
  await next.scrollIntoViewIfNeeded();
  await expect(search).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("button", { name: "查看账号调整明细", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await next.click();
  await search.fill("价格分组 1");
  await expect(page.getByText("价格分组 1", { exact: true })).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("button", { name: "转到上一页", exact: true })).toBeDisabled();
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
});

test("窄屏账号调整预览没有匹配账号时提示在可视区域内", async ({ page }) => {
  await page.goto("/pricing");
  await page.getByRole("button", { name: "查看账号调整明细", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "账号分组调整明细", exact: true });
  await expect(dialog.getByText("当前筛选条件下没有账号", { exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "查看账号调整明细", exact: true })).toBeFocused();
});

test("首次读取失败后结束骨架显示，展示原因且刷新后恢复列表", async ({ page }) => {
  let failed = true;
  await page.route("**/api/pricing", (route) =>
    failed
      ? route.fulfill({ status: 503, json: { detail: "价格服务暂时不可用" } })
      : route.fulfill({ json: catalog }),
  );
  await page.goto("/pricing");
  await expect(page.getByTestId("pricing-load-error")).toContainText("价格服务暂时不可用", {
    timeout: 15000,
  });
  await expect(page.getByTestId("pricing-loading")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "查看账号调整明细", exact: true })).toBeDisabled();
  failed = false;
  await page.getByRole("button", { name: "刷新价格数据" }).click();
  await expect(page.getByText("价格分组 1", { exact: true })).toBeVisible();
  await expect(page.getByTestId("pricing-load-error")).toHaveCount(0);
});

test("搜索无匹配时空提示可见，清空筛选后恢复价格分组", async ({ page }) => {
  await page.goto("/pricing");
  const search = page.getByRole("textbox", { name: "搜索分组、ID 或平台" });
  await search.fill("不存在的分组");
  await expect(page.getByText("没有匹配的分组", { exact: true })).toBeInViewport({ ratio: 1 });
  await search.clear();
  await expect(page.getByText("价格分组 1", { exact: true })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("pricing-catalog.png") });
});

test("首次加载在剩余高度内展示骨架并禁用预览，读取完成后显示列表", async ({ page }) => {
  let completeRead!: () => void;
  const responseReady = new Promise<void>((resolve) => {
    completeRead = resolve;
  });
  await page.route("**/api/pricing", async (route) => {
    await responseReady;
    await route.fulfill({ json: catalog });
  });
  await page.goto("/pricing");
  const loading = page.getByRole("status", { name: "正在读取价格数据" });
  try {
    await expect(loading).toBeVisible();
    await expect(
      page.getByRole("button", { name: "查看账号调整明细", exact: true }),
    ).toBeDisabled();
    const bounds = (await loading.boundingBox())!;
    const content = (await page.locator('[data-slot="page-workspace"]').boundingBox())!;
    expect(bounds.height).toBeLessThanOrEqual(content.height);
  } finally {
    completeRead();
  }
  await expect(loading).toHaveCount(0);
  await expect(page.getByText("价格分组 1", { exact: true })).toBeVisible();
});

test("刷新失败保留已有分组和筛选，超长分组名不挤出表格", async ({ page }) => {
  const name = "超长价格分组".repeat(20);
  await page.route("**/api/pricing", (route) =>
    route.fulfill({ json: { ...catalog, groups: [{ ...catalog.groups[0], name }] } }),
  );
  await page.goto("/pricing");
  const search = page.getByRole("textbox", { name: "搜索分组、ID 或平台" });
  await search.fill("超长");
  await expect(page.getByText(name, { exact: true })).toBeVisible();
  await page.route("**/api/pricing", (route) =>
    route.fulfill({ status: 503, json: { detail: "价格服务暂时不可用" } }),
  );
  await page.getByRole("button", { name: "刷新价格数据" }).click();
  await expect(page.getByText("价格服务暂时不可用", { exact: true })).toBeVisible({
    timeout: 15000,
  });
  await expect(search).toHaveValue("超长");
  await expect(page.getByText(name, { exact: true })).toBeVisible();
  await expect(page.getByTestId("pricing-loading")).toHaveCount(0);
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
});
