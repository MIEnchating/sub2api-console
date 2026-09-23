import { expect, test, type Page, type Route } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { overviewAccount, overviewGroup } from "./fixtures/overview";

async function setupLoading(page: Page, endpoint: string, theme: string | null): Promise<Route[]> {
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
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "加载复查" },
      "/api/groups": [],
      "/api/accounts": [overviewAccount],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [],
      "/api/tasks": [],
      "/api/dictionaries": { items: [] },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  return held;
}

test("320px 低高度视口中加载内容可纵向滚动，分页占位可到达且不横向溢出", async ({
  page,
  colorScheme,
}) => {
  await page.setViewportSize({ width: 320, height: 480 });
  const held = await setupLoading(page, "/api/pricing", colorScheme);
  await page.goto("/pricing");
  await expect(page.getByRole("status", { name: "正在读取价格目录" })).toBeVisible();
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await content.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(page.locator('[data-slot="skeleton-pagination"]')).toBeInViewport({ ratio: 1 });
  await page.screenshot({
    path: test.info().outputPath("short-viewport-loading.png"),
    animations: "disabled",
  });
  for (const route of held) await route.abort();
});

test("自定义检测历史首次读取保持结果网格，完成后正常显示空状态", async ({ page, colorScheme }) => {
  const held = await setupLoading(page, "/api/model-checks/animations", colorScheme);
  await page.goto("/animation-check");
  await page.getByRole("tab", { name: "自定义接口", exact: true }).click();
  const loading = page.getByRole("status", { name: "正在读取自定义检测记录" });
  await expect(loading).toBeVisible();
  expect(await loading.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("custom-history-loading.png"),
    animations: "disabled",
  });
  await expect.poll(() => held.length).toBeGreaterThan(0);
  for (const route of held) await route.fulfill({ json: [] });
  await expect(loading).toHaveCount(0);
  await expect(page.getByText("暂无自定义接口检测记录", { exact: true })).toBeVisible();
});

test("系统设置首次读取后恢复可编辑表单，后台刷新保留表单和输入值", async ({
  page,
  colorScheme,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const held = await setupLoading(page, "/api/config", colorScheme);
  await page.goto("/config");
  await expect(page.getByRole("status", { name: "正在读取连接设置" })).toBeVisible();
  await expect.poll(() => held.length).toBe(1);
  await held[0].fulfill({ json: pageFixtures["/api/config"] });
  const url = page.getByRole("textbox", { name: "Sub2API 地址", exact: true });
  await expect(url).toBeVisible();
  await expect(page.getByRole("status", { name: "正在读取连接设置" })).toHaveCount(0);
  await url.fill("https://edited.example.test");
  await page.getByRole("button", { name: "刷新系统设置", exact: true }).click();
  await expect.poll(() => held.length).toBe(2);
  await expect(
    page.getByRole("button", { name: "刷新系统设置", exact: true }).locator("svg"),
  ).toHaveCSS("animation-name", "none");
  await expect(url).toHaveValue("https://edited.example.test");
  await expect(url).toBeVisible();
  await expect(page.getByRole("status", { name: "正在读取连接设置" })).toHaveCount(0);
  await held[1].fulfill({ json: pageFixtures["/api/config"] });
  await expect(url).toHaveValue("https://edited.example.test");
});

test("概览健康卡片骨架使用实际卡片尺寸与圆角，读取完成后展示分组", async ({
  page,
  colorScheme,
}) => {
  const held = await setupLoading(page, "/api/groups", colorScheme);
  await page.goto("/");
  const loading = page.getByRole("status", { name: "正在读取分组健康" });
  await expect(loading).toBeVisible();
  const placeholder = loading.locator(":scope > div").first();
  const dimensions = await placeholder.evaluate((element) => ({
    height: getComputedStyle(element).minHeight,
    radius: getComputedStyle(element).borderRadius,
  }));
  await page.screenshot({
    path: test.info().outputPath("overview-loading.png"),
    animations: "disabled",
  });
  await expect.poll(() => held.length).toBeGreaterThan(0);
  for (const route of held) await route.fulfill({ json: [{ ...overviewGroup, name: "标准分组" }] });
  const card = page.getByRole("button", { name: "打开分组管理", exact: true });
  await expect(card).toBeVisible();
  await expect(card).toHaveCSS("min-height", dimensions.height);
  await expect(card).toHaveCSS("border-radius", dimensions.radius);
  await expect(loading).toHaveCount(0);
});

test("动画账号首次读取按卡片网格占位，返回后卡片高度及分页位置保持一致", async ({
  page,
  colorScheme,
}) => {
  const held = await setupLoading(page, "/api/accounts", colorScheme);
  await page.goto("/animation-check");
  const region = page.getByRole("region", { name: "动画账号卡片" });
  const loading = region.getByRole("status", { name: "正在读取账号" });
  await expect(loading).toBeVisible();
  const loadingCards = loading.locator(":scope > div");
  await expect(loadingCards).toHaveCount(4);
  const placeholderHeight = (await loadingCards.first().boundingBox())!.height;
  const footer = page.getByRole("navigation", { name: "动画账号分页" });
  const initial = await footer.boundingBox();
  await page.screenshot({
    path: test.info().outputPath("animation-loading.png"),
    animations: "disabled",
  });
  await expect.poll(() => held.length).toBeGreaterThan(0);
  for (const route of held)
    await route.fulfill({
      json: [1, 2, 3, 4].map((id) => ({ ...overviewAccount, id: String(id) })),
    });
  await expect(region.getByRole("article")).toHaveCount(4);
  expect((await region.getByRole("article").first().boundingBox())!.height).toBe(placeholderHeight);
  await expect(loading).toHaveCount(0);
  expect((await footer.boundingBox())!.y).toBe(initial!.y);
});
