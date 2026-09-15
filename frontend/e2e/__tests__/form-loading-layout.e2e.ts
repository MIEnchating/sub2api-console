import { expect, test, type Locator, type Page, type Route } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

const templates = ["默认", "团队"].map((name, index) => ({
  id: String(index + 1),
  revision: 1,
  name,
  priority: 0,
  match: {},
  config: {
    concurrency: 10,
    priority: 0,
    rate_multiplier: "1",
    group_ids: [],
    auto_pause_on_expired: true,
  },
}));
const accounts = ["默认账号", "团队账号"].map((name, index) => ({
  id: String(index + 1),
  name,
  platform: "openai",
  account_type: "oauth",
}));
const fixtures: Record<string, unknown> = {
  ...pageFixtures,
  "/api/setup/status": { initialized: true, configuration_errors: [] },
  "/api/auth/session": { authenticated: true, username: "加载布局测试" },
  "/api/inspection/automation": {
    enabled: false,
    running: false,
    traffic_collection: { enabled: false },
  },
  "/api/dictionaries": { items: [] },
  "/api/accounts": accounts,
  "/api/groups": [],
  "/api/vault": [],
  "/api/account-workbench/templates": templates,
  "/api/account-workbench/maintenance": {
    revision: 1,
    enabled: false,
    interval_minutes: 30,
    cooldown_minutes: 60,
    group_ids: [],
    check_after_repair: false,
    model: "",
  },
  "/api/account-workbench/exports": [],
};

async function holdEndpoint(page: Page, endpoint: string, theme: string | null): Promise<Route[]> {
  const held: Route[] = [];
  await page.addInitScript(
    (value) => localStorage.setItem("sub2api-console-theme", value ?? "light"),
    theme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === endpoint) {
      held.push(route);
      return;
    }
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  return held;
}

async function expectNoOverflow(element: Locator): Promise<void> {
  expect(await element.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
}

const scenarios = [
  {
    tab: "导入账号",
    endpoint: "/api/account-workbench/templates",
    label: "正在读取账号导入配置",
    slot: "skeleton-textarea",
    ready: "#workbench-content",
    columns: false,
  },
  {
    tab: "自动维护",
    endpoint: "/api/account-workbench/maintenance",
    label: "正在读取账号维护设置",
    slot: "maintenance-parameters",
    ready: "#workbench-interval, #workbench-cooldown",
    columns: true,
  },
  {
    tab: "配置模板",
    endpoint: "/api/account-workbench/templates",
    label: "正在读取账号配置模板",
    slot: "workbench-template-grid",
    ready: "article",
    columns: true,
  },
  {
    tab: "账号安全",
    endpoint: "/api/accounts",
    label: "正在读取安全设置账号",
    slot: "security-selectors",
    ready: "#security-account, #security-operation",
    columns: true,
  },
  {
    tab: "私有导出",
    endpoint: "/api/accounts",
    label: "正在读取导出账号",
    slot: "export-account-options",
    ready: 'fieldset[aria-label="可导出账号"] > label',
    columns: true,
  },
];

for (const scenario of scenarios) {
  test(`${scenario.tab}首次读取与真实内容保持列宽和排列，手机不横向溢出`, async ({
    page,
    colorScheme,
  }) => {
    const held = await holdEndpoint(page, scenario.endpoint, colorScheme);
    await page.goto("/account-workbench");
    await page.getByRole("tab", { name: scenario.tab, exact: true }).click();
    const loading = page.getByRole("status", { name: scenario.label, exact: true });
    await expect(loading).toHaveCount(1);
    await expect(loading).toBeVisible();
    const region = loading.locator(`[data-slot="${scenario.slot}"]`);
    const placeholders = scenario.columns ? region.locator(":scope > *") : region;
    const before = await placeholders.first().boundingBox();
    expect(before).not.toBeNull();
    await expectNoOverflow(page.locator('[data-slot="page-content"]'));
    await page.screenshot({
      path: test.info().outputPath(`${scenario.tab}-loading.png`),
      animations: "disabled",
    });
    if (scenario.columns) {
      const second = (await placeholders.nth(1).boundingBox())!;
      if (page.viewportSize()!.width >= 1024) expect(second.y).toBe(before!.y);
      else expect(second.y).toBeGreaterThan(before!.y);
    } else await expect(region).toHaveCSS("height", "256px");
    await expect.poll(() => held.length).toBeGreaterThan(0);
    for (const route of held) await route.fulfill({ json: fixtures[scenario.endpoint] });
    await expect(loading).toHaveCount(0);
    const ready = page.locator('[data-slot="page-content"]').locator(scenario.ready);
    await expect(ready.first()).toBeVisible();
    const after = (await ready.first().boundingBox())!;
    expect(Math.abs(after.width - before!.width)).toBeLessThanOrEqual(1);
    if (scenario.columns) {
      const second = (await ready.nth(1).boundingBox())!;
      if (page.viewportSize()!.width >= 1024) expect(second.y).toBe(after.y);
      else expect(second.y).toBeGreaterThan(after.y);
    } else await expect(ready).toHaveCSS("height", "256px");
    await expectNoOverflow(page.locator('[data-slot="page-content"]'));
    await page.screenshot({
      path: test.info().outputPath(`${scenario.tab}-ready.png`),
      animations: "disabled",
    });
  });
}

test("渠道管理首次读取只出现一张步骤卡，凭据分栏与真实表单一致", async ({ page, colorScheme }) => {
  const endpoint = "/api/newapi/platforms/layout/refresh";
  const held = await holdEndpoint(page, endpoint, colorScheme);
  await page.goto("/newapi/channels");
  const loading = page.getByRole("status", { name: "正在加载渠道管理", exact: true });
  await expect(loading).toBeVisible();
  await expect(loading.locator('[data-slot="card"]')).toHaveCount(1);
  const columns = loading.locator("[data-channel-credentials-layout] > *");
  const before = await Promise.all([columns.nth(0).boundingBox(), columns.nth(1).boundingBox()]);
  await expectNoOverflow(page.locator('[data-slot="page-content"]'));
  await page.screenshot({
    path: test.info().outputPath("channels-loading.png"),
    animations: "disabled",
  });
  await expect.poll(() => held.length).toBe(1);
  await held[0].fulfill({ json: fixtures[endpoint] });
  await expect(loading).toHaveCount(0);
  const ready = page.locator("[data-channel-credentials-layout] > *");
  await expect(page.getByRole("group", { name: "账号凭据", exact: true })).toBeVisible();
  for (let index = 0; index < 2; index++) {
    const after = (await ready.nth(index).boundingBox())!;
    expect(Math.abs(after.width - before[index]!.width)).toBeLessThanOrEqual(1);
  }
  await expectNoOverflow(page.locator('[data-slot="page-content"]'));
  await page.screenshot({
    path: test.info().outputPath("channels-ready.png"),
    animations: "disabled",
  });
});
