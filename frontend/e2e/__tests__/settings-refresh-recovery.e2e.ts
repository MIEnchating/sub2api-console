import { expect, test, type Page } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

const settings = {
  default: {
    models: [],
    concurrency: 35,
    priority: 1,
    load_factor: null,
    pool_mode: false,
    pool_mode_retry_count: 3,
    pool_mode_retry_status_codes: [429],
  },
  groups: [],
  platform_probe_models: {},
};

async function mockShell(page: Page): Promise<string[]> {
  const reads: string[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    reads.push(path);
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "设置测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/config/account-settings": settings,
      "/api/catalog/accounts": [],
      "/api/catalog/groups": [],
      "/api/dictionaries": { items: [] },
    };
    if (path.endsWith("/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    if (route.request().method() === "GET" && path in fixtures)
      return route.fulfill({ json: fixtures[path] });
    return route.fulfill({ status: 503, json: { detail: "未配置的隔离接口" } });
  });
  return reads;
}

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

test("账号设置首次失败可用键盘重试，成功后显示真实配置且没有占位表单", async ({ page }) => {
  await mockShell(page);
  let failed = true;
  await page.route("**/api/config/account-settings", (route) =>
    failed
      ? route.fulfill({ status: 503, json: { detail: "设置暂不可读，请重试" } })
      : route.fallback(),
  );
  await page.goto("/config?tab=accounts");
  const retry = page.getByRole("button", { name: "重新读取", exact: true });
  // 503 follows the shared three-retry backoff before exposing manual recovery.
  await expect(retry).toBeVisible({ timeout: 15000 });
  await expect(page.getByRole("button", { name: "保存全局默认" })).toHaveCount(0);
  failed = false;
  await retry.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("spinbutton", { name: "全局默认 并发上限" })).toHaveValue("35");
  await expect(retry).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("settings-recovered.png") });
});

test("账号页刷新等待当前设置，保留编辑草稿并避免读取其他设置状态", async ({ page }) => {
  const reads = await mockShell(page);
  await page.goto("/config?tab=accounts");
  const concurrency = page.getByRole("spinbutton", { name: "全局默认 并发上限" });
  await expect(concurrency).toHaveValue("35");
  await concurrency.fill("47");
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/config/account-settings", async (route) => {
    await gate;
    await route.fulfill({
      json: { ...settings, default: { ...settings.default, concurrency: 44 } },
    });
  });
  try {
    const refresh = page.getByRole("button", { name: "刷新系统设置" });
    await expect(refresh).toBeEnabled();
    reads.length = 0;
    const reading = page.waitForRequest("**/api/config/account-settings");
    await refresh.click();
    await reading;
    await expect(refresh).toBeDisabled();
    await expect(concurrency).toHaveValue("47");
    release();
    await expect(refresh).toBeEnabled();
    await expect(concurrency).toHaveValue("47");
    expect(reads).toContain("/api/config/model-sync");
    expect(reads).toContain("/api/catalog/groups");
    expect(reads).not.toContain("/api/notifications/status");
    expect(reads).not.toContain("/api/config/log-cleanup");
    await page.screenshot({ path: test.info().outputPath("settings-draft.png") });
  } finally {
    release();
  }
});
