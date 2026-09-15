import { expect, test, type Locator, type Page, type Route } from "@playwright/test";

import { pricing } from "./fixtures/settings";

async function holdPricing(page: Page, theme: string | null): Promise<Route[]> {
  const held: Route[] = [];
  await page.addInitScript(
    (value) => localStorage.setItem("sub2api-console-theme", value ?? "light"),
    theme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/pricing") {
      held.push(route);
      return;
    }
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价格加载测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": { items: [] },
      "/api/pricing/backups": [],
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  return held;
}

async function expectNoHorizontalOverflow(element: Locator): Promise<void> {
  expect(await element.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
}

test("价格设置首次读取只显示一套配置布局，骨架列宽与读取后的卡片一致", async ({
  page,
  colorScheme,
}) => {
  const held = await holdPricing(page, colorScheme);
  await page.goto("/pricing-config");
  const loading = page.getByRole("status", { name: "正在读取价格设置", exact: true });
  await expect(loading).toHaveCount(1);
  await expect(loading).toBeVisible();
  await expect(loading).toHaveAttribute("aria-busy", "true");
  await expect(page.getByRole("button", { name: "保存配置", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "立即调整", exact: true })).toBeDisabled();
  const settings = page.getByTestId("pricing-settings-skeleton");
  const exchange = page.getByTestId("pricing-exchange-skeleton");
  await expect(settings).toBeVisible();
  await expect(exchange).toBeVisible();
  const settingsBounds = (await settings.boundingBox())!;
  const exchangeBounds = (await exchange.boundingBox())!;
  if (page.viewportSize()!.width >= 1280) {
    expect(settingsBounds.width).toBe(288);
    expect(exchangeBounds.width).toBeGreaterThan(settingsBounds.width);
    expect(exchangeBounds.y).toBe(settingsBounds.y);
  } else {
    expect(exchangeBounds.width).toBe(settingsBounds.width);
    expect(exchangeBounds.y).toBeGreaterThanOrEqual(settingsBounds.y + settingsBounds.height);
  }
  for (const control of await loading.locator('[data-slot="skeleton-control"]').all()) {
    await expect(control).toHaveCSS("height", "32px");
  }
  await expectNoHorizontalOverflow(page.locator('[data-slot="page-content"]'));
  await expectNoHorizontalOverflow(loading);
  await page.screenshot({ path: test.info().outputPath("pricing-config-loading.png") });
  await expect.poll(() => held.length).toBe(1);
  await held[0].fulfill({ json: pricing });
  await expect(loading).toHaveCount(0);
  const readySettings = page.getByTestId("pricing-settings-panel");
  const readyExchange = page.getByRole("region", { name: /^账号互换范围/ });
  await expect(readySettings).toBeVisible();
  await expect(readyExchange).toBeVisible();
  expect((await readySettings.boundingBox())!.width).toBe(settingsBounds.width);
  expect((await readyExchange.boundingBox())!.width).toBe(exchangeBounds.width);
  await expect(page.getByRole("button", { name: "保存配置", exact: true })).toBeEnabled();
  await expectNoHorizontalOverflow(page.locator('[data-slot="page-content"]'));
});

test("中等桌面宽度读取价格设置时上下排列，设置字段保持三列", async ({
  page,
  colorScheme,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-light", "中间断点只需验证一次");
  await page.setViewportSize({ width: 1100, height: 900 });
  const held = await holdPricing(page, colorScheme);
  await page.goto("/pricing-config");
  const settings = page.getByTestId("pricing-settings-skeleton");
  const exchange = page.getByTestId("pricing-exchange-skeleton");
  await expect(settings).toBeVisible();
  await expect(exchange).toBeVisible();
  const settingsBounds = (await settings.boundingBox())!;
  const exchangeBounds = (await exchange.boundingBox())!;
  expect(exchangeBounds.y).toBeGreaterThanOrEqual(settingsBounds.y + settingsBounds.height);
  expect(exchangeBounds.width).toBe(settingsBounds.width);
  const controls = settings.locator('[data-slot="skeleton-control"]');
  await expect(controls).toHaveCount(3);
  const first = (await controls.nth(0).boundingBox())!;
  const last = (await controls.nth(2).boundingBox())!;
  expect(last.y).toBe(first.y);
  expect(last.x).toBeGreaterThan(first.x);
  await expectNoHorizontalOverflow(page.locator('[data-slot="page-content"]'));
  await expect.poll(() => held.length).toBe(1);
  await held[0].fulfill({ json: pricing });
  await expect(page.getByRole("status", { name: "正在读取价格设置" })).toHaveCount(0);
  const inputs = page.getByTestId("pricing-settings-panel").getByRole("spinbutton");
  const readyFirst = (await inputs.nth(0).boundingBox())!;
  const readyLast = (await inputs.nth(2).boundingBox())!;
  expect(readyLast.y).toBe(readyFirst.y);
  expect(readyLast.x).toBeGreaterThan(readyFirst.x);
});

test("价格设置后台刷新保留已加载配置和未保存的输入，不重新出现骨架", async ({
  page,
  colorScheme,
}) => {
  const held = await holdPricing(page, colorScheme);
  await page.goto("/pricing-config");
  await expect.poll(() => held.length).toBe(1);
  await held[0].fulfill({ json: pricing });
  const margin = page.getByRole("spinbutton", { name: "目标盈利比例", exact: true });
  await expect(margin).toHaveValue("20");
  await margin.fill("35");
  await page.getByRole("button", { name: "刷新价格数据", exact: true }).click();
  await expect.poll(() => held.length).toBe(2);
  await expect(page.getByRole("status", { name: "正在读取价格设置" })).toHaveCount(0);
  await expect(page.getByTestId("pricing-settings-panel")).toBeVisible();
  await expect(page.getByRole("region", { name: /^账号互换范围/ })).toBeVisible();
  await expect(margin).toHaveValue("35");
  await held[1].fulfill({ json: pricing });
  await expect(page.getByRole("button", { name: "刷新价格数据", exact: true })).toBeEnabled();
  await expect(margin).toHaveValue("35");
});
