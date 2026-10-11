import { expect, test, type Page, type Request, type Route } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

const rawPath = "/api/newapi/platforms/layout/remote-model-prices/raw";
const source = {
  content: '{"fresh-model":{"input_cost_per_token":0.000000123456789}}',
  source_url: "https://pricing.example.test/catalog.json",
  fetched_at: "2026-10-10T00:00:00Z",
  size_bytes: 66,
  sha256: "a".repeat(64),
};

async function holdReads(page: Page, path: string): Promise<Map<Request, Route>> {
  const active = new Map<Request, Route>();
  page.on("requestfailed", (request) => active.delete(request));
  await page.route(`**${path}`, (route) => {
    if (!route.request().failure()) active.set(route.request(), route);
  });
  return active;
}

test.beforeEach(async ({ page, colorScheme, viewport }) => {
  if (viewport!.width < 640) await page.setViewportSize({ width: 320, height: 568 });
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价格读取" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": { items: [] },
      [rawPath]: source,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离接口未配置" } });
  });
});

test("关闭读取中的原始价卡会中止网络请求并恢复焦点，重开读取新内容", async ({ page }) => {
  const active = await holdReads(page, rawPath);
  await page.goto("/newapi/prices");
  const trigger = page.getByRole("button", { name: "查看原始价卡", exact: true });
  await trigger.click();
  await expect(page.getByRole("status", { name: "正在读取远程价卡原始文件" })).toBeVisible();
  await expect.poll(() => active.size).toBe(1);
  const request = [...active.keys()][0];
  const cancelled = page.waitForEvent("requestfailed", {
    predicate: (failed) => failed === request,
  });
  await page.keyboard.press("Escape");
  await cancelled;
  await expect(trigger).toBeFocused();
  await page.route(`**${rawPath}`, (route) => route.fulfill({ json: source }));
  await trigger.click();
  await expect(page.getByRole("button", { name: "查看 fresh-model 明细" })).toBeVisible();
  await expect(page.getByText("远程价卡原始文件读取失败", { exact: true })).toHaveCount(0);
  await page.screenshot({ path: test.info().outputPath("raw-pricing-reopened.png") });
});

test("原始价卡失败显示单次悬浮提示，键盘重试成功后允许关闭", async ({ page }) => {
  let failed = true;
  await page.route(`**${rawPath}`, (route) =>
    route.fulfill(
      failed ? { status: 502, json: { detail: "价卡源暂时不可用，请重试" } } : { json: source },
    ),
  );
  await page.goto("/newapi/prices");
  await page.getByRole("button", { name: "查看原始价卡", exact: true }).click();
  await expect(page.getByText("价卡源暂时不可用，请重试", { exact: true })).toHaveCount(1);
  const retry = page.getByRole("button", { name: "重新读取", exact: true });
  await retry.focus();
  failed = false;
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button", { name: "查看 fresh-model 明细" })).toBeVisible();
  await page
    .getByRole("dialog", { name: "远程价卡原始文件" })
    .getByRole("button", { name: "关闭", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

for (const endpoint of ["refresh", "management-model-prices"]) {
  test(`${endpoint} 读取中离开价格页会中止请求`, async ({ page }) => {
    const active = await holdReads(page, `/api/newapi/platforms/layout/${endpoint}`);
    await page.goto("/newapi/prices");
    if (endpoint === "management-model-prices")
      await page.getByRole("tab", { name: "远程模型价格", exact: true }).click();
    else await expect(page.getByRole("status", { name: "正在加载模型价格" })).toBeVisible();
    await expect.poll(() => active.size).toBe(1);
    const request = [...active.keys()][0];
    const cancelled = page.waitForEvent("requestfailed", {
      predicate: (failed) => failed === request,
    });
    if (page.viewportSize()!.width < 640)
      await page.getByRole("button", { name: "切换侧边栏" }).click();
    await page.getByRole("link", { name: "账号模型", exact: true }).click();
    await cancelled;
    await expect(page).toHaveURL(/account-model-automation/);
  });
}
