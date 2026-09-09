import { expect, test } from "@playwright/test";
import type { GroupStatus, PricingSnapshot } from "../../src/api";

const group: GroupStatus = {
  id: "6",
  name: "顶部操作测试分组",
  platform: "openai",
  account_count: 1,
  scheduling_open: 1,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "balanced",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "healthy",
  override: null,
};
const pricing: PricingSnapshot = {
  config: {
    enabled: false,
    profit_margin: 0.2,
    interval_seconds: 120,
    write_concurrency: 4,
    exchange_group_sets: [],
    exchange_group_set_names: [],
  },
  groups: [],
  decisions: [],
  accounts: 0,
  changes: 0,
  skipped: 0,
  generated_at: "2026-09-09T00:00:00Z",
};

test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "顶部操作回归测试" },
      "/api/config": { mode: "完全模式", values: {} },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/overview": {
        database_available: true,
        account_count: 1,
        group_count: 1,
        open_alerts: 0,
        recent_runs: 0,
        last_activity: null,
        mode: "完全模式",
      },
      "/api/groups": [group],
      "/api/accounts": [],
      "/api/policy": {
        available: true,
        global_strategy: "balanced",
        advanced_policy: {},
        probe_interval_seconds: 300,
        configuration_errors: [],
      },
      "/api/auth-recovery/config": { auth_records: [], vault_entries: [] },
      "/api/upstreams": {
        hosts: [],
        total_hosts: 0,
        authenticated_hosts: 0,
        recovery_required: 0,
        source: "test",
      },
      "/api/pricing": pricing,
      "/api/pricing/backups": [
        {
          id: "backup-1",
          name: "测试备份",
          actor: "tester",
          account_count: 1,
          created_at: "2026-09-09T00:00:00Z",
        },
      ],
      "/api/pricing/changes": [],
      "/api/pricing/revenue/latest": null,
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (route.request().method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

for (const fixture of [
  {
    route: "/groups",
    title: "分组管理",
    menu: "分组维护",
    labels: ["回落到全局策略", "排除分组", "恢复管控"],
    count: 2,
  },
  {
    route: "/upstreams",
    title: "上游管理",
    menu: "上游维护",
    labels: ["统计变化", "同步上游", "同步余额", "名称修复", "同步分组", "核对分组绑定"],
    count: 3,
  },
  {
    route: "/pricing",
    title: "价格管理",
    menu: "价格维护",
    labels: ["变更记录", "创建备份", "从备份还原"],
    count: 3,
  },
]) {
  test(`${fixture.title}顶部保留紧凑操作，维护菜单可用鼠标和键盘打开`, async ({ page }) => {
    await page.goto(fixture.route);
    const heading = page.locator('[data-slot="page-heading"]');
    await expect(heading.getByRole("heading", { name: fixture.title })).toBeVisible();
    const buttons = heading.getByRole("button");
    await expect(buttons).toHaveCount(fixture.count);
    const positions = await buttons.evaluateAll((elements) =>
      elements.map((element) => {
        const bounds = element.getBoundingClientRect();
        return { top: bounds.top, bottom: bounds.bottom };
      }),
    );
    expect(Math.max(...positions.map((position) => position.top))).toBeLessThan(
      Math.min(...positions.map((position) => position.bottom)),
    );
    expect(
      await heading
        .getByRole("heading")
        .evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    const trigger = heading.getByRole("button", { name: fixture.menu });
    await trigger.click();
    const menu = page.getByRole("menu");
    await expect(menu).toBeVisible();
    for (const label of fixture.labels) {
      await expect(menu.getByRole("menuitem", { name: label, exact: true })).toBeInViewport({
        ratio: 1,
      });
      await expect(heading.getByRole("button", { name: label, exact: true })).toHaveCount(0);
    }
    await page.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    await page.keyboard.press("Enter");
    await expect(menu).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await page.screenshot({
      path: test.info().outputPath(`${fixture.route.slice(1)}-actions.png`),
    });
  });
}

for (const fixture of [
  { label: "变更记录", dialog: "价格分组变更记录" },
  { label: "创建备份", dialog: "创建价格分组备份" },
  { label: "从备份还原", dialog: "从备份还原分组" },
]) {
  test(`价格维护中的${fixture.label}可打开原有窗口`, async ({ page }) => {
    await page.goto("/pricing");
    await page.getByRole("button", { name: "价格维护" }).click();
    const action = page.getByRole("menuitem", { name: fixture.label, exact: true });
    await expect(action).toBeEnabled();
    await action.click();
    await expect(page.getByRole("dialog", { name: fixture.dialog })).toBeVisible();
    await expect(page.getByRole("menu")).not.toBeVisible();
  });
}

test("分组维护先确认当前筛选范围，取消后仍可使用页面", async ({ page }) => {
  await page.goto("/groups");
  await page.getByRole("button", { name: "分组维护" }).click();
  await expect(page.getByText("当前筛选 1 个分组")).toBeVisible();
  await page.getByRole("menuitem", { name: "排除分组", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "批量排除分组" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("list", { name: "本次处理的分组" })).toContainText(
    `${group.name}（#6）`,
  );
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("button", { name: "分组维护" })).toBeEnabled();
});

for (const fixture of [
  { route: "/onboarding", title: "账号添加" },
  { route: "/revenue-analysis", title: "收益分析" },
  { route: "/pricing-config", title: "价格配置" },
]) {
  test(`${fixture.title}顶部控件在窄屏也保持同一行并保留可访问名称`, async ({ page }) => {
    await page.goto(fixture.route);
    const heading = page.locator('[data-slot="page-heading"]');
    await expect(heading.getByRole("heading", { name: fixture.title })).toBeVisible();
    const buttons = heading.getByRole("button");
    const positions = await buttons.evaluateAll((elements) =>
      elements.map((element) => {
        const bounds = element.getBoundingClientRect();
        return {
          top: bounds.top,
          bottom: bounds.bottom,
          label: element.getAttribute("aria-label") || element.textContent?.trim(),
        };
      }),
    );
    expect(positions.length).toBeGreaterThan(0);
    expect(positions.every((position) => Boolean(position.label))).toBe(true);
    expect(Math.max(...positions.map((position) => position.top))).toBeLessThan(
      Math.min(...positions.map((position) => position.bottom)),
    );
    expect(
      await heading
        .getByRole("heading")
        .evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });
}
