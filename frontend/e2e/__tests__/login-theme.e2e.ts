import { expect, test, type Page } from "@playwright/test";

async function expectSharedTheme(page: Page): Promise<void> {
  const body = page.locator("body");
  const theme = await body.evaluate((element) => {
    const styles = getComputedStyle(element);
    return { background: styles.backgroundColor, font: styles.fontFamily };
  });
  await expect(page.locator(".login-shell")).toHaveCSS("background-color", theme.background);
  await expect(page.locator(".login-shell")).toHaveCSS("font-family", theme.font);
  const submit = page.getByRole("button", { name: "登录", exact: true });
  const illustration = page.getByRole("img", { name: /^Console 统一管理 Sub2API/ });
  const signal = illustration.locator(".login-map-signals path").first();
  await expect
    .poll(async () => {
      const primary = await submit.evaluate((element) => getComputedStyle(element).backgroundColor);
      const stroke = await signal.evaluate((element) => getComputedStyle(element).stroke);
      return stroke === primary;
    })
    .toBe(true);
}

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/setup/status")
      return route.fulfill({ json: { initialized: true, configuration_errors: [] } });
    if (path === "/api/auth/session") return route.fulfill({ json: { authenticated: false } });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/");
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeVisible();
});

test("登录页使用控制台的背景字体、页眉高度与标准输入框尺寸", async ({ page }) => {
  await expectSharedTheme(page);
  await expect(page.locator("header")).toHaveCSS("height", "48px");
  const username = page.getByRole("textbox", { name: "账号", exact: true });
  const password = page.getByLabel("密码", { exact: true });
  const reveal = page.getByRole("button", { name: "显示密码", exact: true });
  for (const control of [username, password, reveal])
    await expect(control).toHaveCSS("height", "32px");
  const inputBounds = await password.boundingBox();
  const revealBounds = await reveal.boundingBox();
  expect(revealBounds!.y).toBe(inputBounds!.y);
  expect(revealBounds!.x + revealBounds!.width).toBeLessThanOrEqual(
    inputBounds!.x + inputBounds!.width,
  );
});

test("更换全局配色字体和圆角后登录页与管理关系图同步更新", async ({ page }) => {
  await page.locator("body").evaluate((element) => {
    element.setAttribute("data-theme-preset", "rose-garden");
    element.setAttribute("data-theme-font", "serif");
    element.setAttribute("data-theme-radius", "sm");
  });
  await expectSharedTheme(page);
  const illustration = page.getByRole("img", { name: /^Console 统一管理 Sub2API/ });
  const radius = await page
    .getByRole("button", { name: "登录", exact: true })
    .evaluate((element) => getComputedStyle(element).borderRadius);
  if ((page.viewportSize()?.width ?? 0) < 1024) {
    await expect(illustration.locator(".login-map-mobile-console")).toHaveCSS(
      "border-radius",
      radius,
    );
  } else {
    await expect(illustration.locator(".login-map-center")).toHaveCSS("rx", radius);
  }
  await page.screenshot({ path: test.info().outputPath("login-custom-theme.png"), fullPage: true });
});
