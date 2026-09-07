import { expect, test } from "@playwright/test";

const groups = Array.from({ length: 30 }, (_, index) => ({
  id: String(index + 1),
  name: `分组${index + 1}-${"long-group-name-".repeat(4)}`,
}));

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    window.localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  // Every API request is intercepted, including writes and SSE. No backend is used.
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "样式审查" },
      "/api/overview": {
        mode: "监控模式",
        database_available: true,
        account_count: 0,
        group_count: 30,
        open_alerts: 0,
        recent_runs: 0,
        last_activity: null,
      },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/groups": groups,
      "/api/logs": { items: [], total: 0, page: 1, page_size: 20, truncated: false },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: ": isolated fixture\n\n",
      });
      return;
    }
    if (route.request().method() !== "GET" || !(path in responses)) {
      await route.fulfill({
        status: 503,
        json: { code: "isolated_test", detail: "隔离测试未配置此接口" },
      });
      return;
    }
    await route.fulfill({ json: responses[path] });
  });
});

test("uses the loaded font and keeps the page inside the viewport", async ({ page }) => {
  await page.goto("/logs?kind=event");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.locator("body")).toHaveCSS("font-family", /Public Sans Variable/);
  await expect(page.getByText("暂无日志记录", { exact: true })).toBeInViewport({ ratio: 1 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("logs.png") });
});

test("keeps filter options scrollable on short screens and reveals keyboard selection", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 480 });
  await page.goto("/logs?kind=event");
  await page.getByRole("button", { name: "事件分组筛选" }).click();
  const search = page.getByRole("combobox", { name: "搜索事件分组" });
  await expect(search).toBeFocused();
  await search.press("End");
  const lastOption = page.getByRole("option").last();
  await expect(lastOption).toBeInViewport();
  const popup = page.locator('[data-slot="faceted-filter-content"]');
  const bounds = await popup.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.y).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(480);
  await search.press("Enter");
  await expect(lastOption).toHaveAttribute("aria-selected", "true");
  await expect(lastOption).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: test.info().outputPath("filter.png") });
  await search.press("Escape");
  await expect(page.getByRole("button", { name: "事件分组筛选" })).toBeFocused();
});

test("keeps a pressed toolbar command in its resting position", async ({ page }) => {
  await page.goto("/logs?kind=event");
  const button = page.getByRole("button", { name: "事件分组筛选" });
  await button.hover();
  const before = await button.boundingBox();
  await page.mouse.down();
  await expect(button).toHaveCSS("transform", "none");
  await expect(button).toHaveCSS("translate", "none");
  expect(await button.boundingBox()).toEqual(before);
  await page.mouse.up();
});

test("moves keyboard focus into the mobile navigation sheet and restores it on close", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 664 });
  await page.goto("/logs?kind=event");
  const trigger = page.getByRole("button", { name: "切换侧边栏" });
  await trigger.focus();
  await trigger.press("Enter");
  const sheet = page.getByRole("dialog", { name: "导航菜单" });
  await expect(sheet).toBeVisible();
  await expect
    .poll(() => sheet.evaluate((element) => element.contains(document.activeElement)))
    .toBe(true);
  await page.keyboard.press("Escape");
  await expect(sheet).not.toBeVisible();
  await expect(trigger).toBeFocused();
});

test("keeps primary command text readable at rest and on hover", async ({ page }) => {
  await page.goto("/logs?kind=event");
  const button = page.getByRole("button", { name: "启动自动调度" });
  await expect(button).toBeEnabled();
  for (const hover of [false, true]) {
    if (hover) await button.hover();
    await expect
      .poll(() =>
        button.evaluate((element) => {
          const context = document.createElement("canvas").getContext("2d");
          if (!context) throw new Error("Canvas is unavailable");
          const style = getComputedStyle(element);
          const luminance = (color: string): number => {
            context.clearRect(0, 0, 1, 1);
            context.fillStyle = getComputedStyle(document.body).backgroundColor;
            context.fillRect(0, 0, 1, 1);
            context.fillStyle = color;
            context.fillRect(0, 0, 1, 1);
            return Array.from(context.getImageData(0, 0, 1, 1).data)
              .slice(0, 3)
              .map((value) => value / 255)
              .map((value) => (value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4))
              .reduce((total, value, index) => total + value * [0.2126, 0.7152, 0.0722][index], 0);
          };
          const foreground = luminance(style.color);
          const background = luminance(style.backgroundColor);
          return (
            (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05)
          );
        }),
      )
      .toBeGreaterThanOrEqual(4.5);
  }
});
