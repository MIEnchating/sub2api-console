import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  // Freeze the effect in the stylesheet before rendering, without racing its
  // short duration or adding a style node before the HTML parser builds <head>.
  await page.route("**/*.css", async (route) => {
    const response = await route.fetch();
    await route.fulfill({
      response,
      body:
        (await response.text()) + "\n.console-page { animation-play-state: paused !important; }",
    });
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "页面切换测试" },
      "/api/overview": { open_alerts: 0 },
      "/api/groups": [],
      "/api/inspection/automation": { enabled: false, running: false },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("页面入场保持短淡入、位移和模糊效果，结束后内容可见且不溢出", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.goto("/logs");
  const content = page.locator(".console-page");
  await expect(content).toHaveCSS("opacity", "0");
  await expect(content).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 8)");
  await expect(content).toHaveCSS("filter", "blur(4px)");
  const duration = await content.evaluate((element) => {
    const animations = element.getAnimations();
    if (animations.length !== 1) throw new Error("页面应只有一个入场效果");
    const duration = animations[0].effect?.getTiming().duration;
    animations[0].finish();
    return duration;
  });
  expect(duration).toBe(150);
  await expect(content).toHaveCSS("opacity", "1");
  await expect(content).toHaveCSS("transform", "none");
  await expect(content).toHaveCSS("filter", "none");
  await expect(page.getByText("暂无日志记录", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("route-transition.png") });
});

test("开启减少动态效果时直接展示页面，并随系统偏好变化停止动画", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/logs");
  const content = page.locator(".console-page");
  await expect(content).toHaveCSS("opacity", "1");
  expect(await content.evaluate((element) => element.getAnimations().length)).toBe(0);
  await expect(page.getByText("暂无日志记录", { exact: true })).toBeVisible();
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await expect(content).toHaveCSS("opacity", "0");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(content).toHaveCSS("opacity", "1");
  await expect(content).toHaveCSS("transform", "none");
  await expect(content).toHaveCSS("filter", "none");
  expect(await content.evaluate((element) => element.getAnimations().length)).toBe(0);
});
