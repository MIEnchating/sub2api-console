import { expect, test, type Route } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("首次打开动画检测等待代码下载时显示卡片骨架，加载完成后切换到账号内容", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const held: Route[] = [];
  await page.route("**/*animation-check-panel*.js", (route) => {
    held.push(route);
  });
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "加载复查" },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/model-checks/animations": [],
      "/api/model-checks/animation-schedules": [],
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    if (path in fixtures) return route.fulfill({ json: fixtures[path] });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check", { waitUntil: "domcontentloaded" });
  await expect.poll(() => held.length).toBeGreaterThan(0);
  const loading = page.getByRole("status", { name: "正在读取动画检测" });
  await expect(loading).toBeVisible();
  await expect(loading.locator('[data-slot="skeleton"]')).not.toHaveCount(0);
  await expect(loading.locator("svg")).toHaveCount(0);
  expect(await loading.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("animation-module-loading.png") });
  await page.unroute("**/*animation-check-panel*.js");
  for (const route of held) await route.continue();
  await expect(loading).toHaveCount(0);
  await expect(page.getByRole("tab", { name: "账号检测", exact: true })).toBeVisible();
});
