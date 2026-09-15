import { expect, test, type Route } from "@playwright/test";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

test("启动读取完成进入初始化时始终显示统一品牌图标", async ({ page }) => {
  let pendingSetup: Route | undefined;
  await page.route("**/api/**", (route) => {
    if (new URL(route.request().url()).pathname === "/api/setup/status") {
      pendingSetup = route;
      return;
    }
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });

  await page.goto("/");
  const loading = page.getByRole("status", { name: "正在读取初始化状态…", exact: true });
  await expect(loading).toHaveAttribute("aria-busy", "true");
  await expect(loading.locator('img[src="/console-mark.svg"]')).toBeInViewport({ ratio: 1 });

  await expect.poll(() => Boolean(pendingSetup)).toBe(true);
  await pendingSetup!.fulfill({
    json: { initialized: false, target_configured: true, configuration_errors: [] },
  });
  await expect(page.getByText("初始化控制台", { exact: true })).toBeVisible();
  await expect(page.locator('img[src="/console-mark.svg"]')).toBeInViewport({ ratio: 1 });
  await expect(loading).toHaveCount(0);
  await page.getByRole("textbox", { name: "控制台账号", exact: true }).fill("operator");
  await expect(page.getByRole("textbox", { name: "控制台账号", exact: true })).toHaveValue(
    "operator",
  );
  await page.screenshot({ path: test.info().outputPath("setup-brand.png"), fullPage: true });
});

test("初始化读取失败后保留品牌且可重试进入登录页", async ({ page }) => {
  let unavailable = true;
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/setup/status") {
      if (unavailable) return route.fulfill({ status: 503, json: { detail: "连接暂时不可用" } });
      return route.fulfill({ json: { initialized: true, configuration_errors: [] } });
    }
    if (path === "/api/auth/session") return route.fulfill({ json: { authenticated: false } });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });

  await page.goto("/");
  const retry = page.getByRole("button", { name: "重新连接", exact: true });
  await expect(retry).toBeVisible();
  await expect(page.locator('img[src="/console-mark.svg"]')).toBeInViewport({ ratio: 1 });

  unavailable = false;
  await retry.click();
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeVisible();
  await expect(page.locator('header img[src="/console-mark.svg"]')).toBeInViewport({ ratio: 1 });
});
