import { expect, test, type Locator, type Page, type Route } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

const templates = ["默认", "团队"].map((name, index) => ({
  id: String(index + 1),
  revision: 1,
  name,
  source_id: "",
  source_name: "",
  source_version: "",
  synced_at: "2026-09-18T00:00:00Z",
  summary: { proxy_name: "", groups: [] },
  config: {
    credential_extras: {},
    extra: {},
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
  "/api/account-workbench/templates": { revision: 1, preferred_id: "", items: templates },
  "/api/account-workbench/accounts": [],
  "/api/newapi/platforms/layout/channel-groups": { version: "1", groups: [] },
  "/api/newapi/platforms/layout/channels": { items: [], total: 0 },
  "/api/account-workbench/maintenance": {
    revision: 1,
    running: false,
    results: [],
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

test("模板读取期间保留账号输入，模板选择禁用且读取完成后恢复", async ({ page, colorScheme }) => {
  const endpoint = "/api/account-workbench/templates";
  const held = await holdEndpoint(page, endpoint, colorScheme);
  await page.goto("/account-workbench");
  const input = page.getByRole("textbox", { name: "账号资料", exact: true });
  await input.fill("rt_isolated");
  const selector = page.getByRole("combobox", { name: "配置模板" });
  await expect(selector).toBeDisabled();
  await expectNoOverflow(page.getByRole("tabpanel"));
  await expect.poll(() => held.length).toBeGreaterThan(0);
  for (const route of held) await route.fulfill({ json: fixtures[endpoint] });
  await expect(selector).toBeEnabled();
  await expect(input).toHaveValue("rt_isolated");
  await expectNoOverflow(page.getByRole("tabpanel"));
});

for (const scenario of [
  {
    tab: "自动维护",
    endpoint: "/api/account-workbench/maintenance",
    loading: "正在读取维护设置",
    ready: "维护设置",
    skeletons: 3,
  },
  {
    tab: "配置模板",
    endpoint: "/api/account-workbench/templates",
    loading: "正在读取模板",
    ready: "模板列表",
    skeletons: 2,
  },
]) {
  test(`${scenario.tab}首次读取展示骨架，完成后恢复操作且窄屏不溢出`, async ({
    page,
    colorScheme,
  }) => {
    const held = await holdEndpoint(page, scenario.endpoint, colorScheme);
    await page.goto("/account-workbench");
    await page.getByRole("tab", { name: scenario.tab, exact: true }).click();
    const loading = page.getByLabel(scenario.loading, { exact: true });
    await expect(loading).toHaveAttribute("aria-busy", "true");
    await expect(loading.locator('[data-slot="skeleton"]')).toHaveCount(scenario.skeletons);
    await expectNoOverflow(page.getByRole("tabpanel"));
    await expect.poll(() => held.length).toBeGreaterThan(0);
    for (const route of held) await route.fulfill({ json: fixtures[scenario.endpoint] });
    await expect(loading).toHaveCount(0);
    const ready = page.getByLabel(scenario.ready, { exact: true });
    await expect(ready).toBeVisible();
    if (scenario.tab === "配置模板") {
      await expect(ready.getByRole("article")).toHaveCount(2);
      await expect(page.getByRole("button", { name: "创建模板", exact: true })).toBeEnabled();
    } else await expect(page.getByRole("button", { name: "预览并保存" })).toBeEnabled();
    await expectNoOverflow(page.getByRole("tabpanel"));
  });
}

test("渠道列表读取期间保留筛选，完成后展示空状态且不加载新增表单", async ({
  page,
  colorScheme,
}) => {
  const endpoint = "/api/newapi/platforms/layout/channels";
  const held = await holdEndpoint(page, endpoint, colorScheme);
  await page.goto("/newapi/channels");
  const loading = page.getByRole("status", { name: "正在读取现有渠道" });
  await expect(loading).toBeVisible();
  await expect(loading).toHaveAttribute("aria-busy", "true");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expectNoOverflow(page.locator('[data-slot="page-content"]'));
  await expect.poll(() => held.length).toBeGreaterThan(0);
  for (const route of held) await route.fulfill({ json: fixtures[endpoint] });
  await expect(loading).toHaveCount(0);
  await expect(page.getByText("暂无渠道", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "新增渠道" })).toBeEnabled();
  await expectNoOverflow(page.locator('[data-slot="page-content"]'));
});
