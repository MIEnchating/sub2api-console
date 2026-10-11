import { expect, test } from "@playwright/test";

test.beforeEach(async ({ context }) => {
  await context.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/setup/status") return route.fulfill({ json: { initialized: true } });
    if (path === "/api/auth/session") return route.fulfill({ json: { authenticated: false } });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
});

for (const theme of ["light", "dark"] as const) {
  test(`应用脚本尚未加载时已应用保存的${theme}主题`, async ({ page }) => {
    await page.addInitScript(
      (value) => localStorage.setItem("sub2api-console-theme", value),
      theme,
    );
    await page.route("**/*", (route) => {
      if (route.request().resourceType() === "script") return route.abort();
      return route.fallback();
    });
    await page.goto("/", { waitUntil: "domcontentloaded" });
    await expect(page.locator("#root")).toBeEmpty();
    await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
    await expect(page.locator("html")).toHaveCSS("color-scheme", theme);
    expect(
      await page.locator("html").evaluate((element) => element.classList.contains("dark")),
    ).toBe(theme === "dark");
    await expect(page.locator('meta[name="theme-color"]')).toHaveAttribute(
      "content",
      theme === "light" ? "#ffffff" : "#1e1e1e",
    );
  });
}

test("同源标签页切换主题后其他页面同步，清除偏好回退深色且不重新写回存储", async ({
  context,
  page,
}) => {
  await page.goto("/");
  await expect(page.getByRole("button", { name: "切换亮色主题" })).toBeVisible();
  const other = await context.newPage();
  await other.goto("/");
  await expect(other.getByRole("button", { name: "切换亮色主题" })).toBeVisible();
  await page.getByRole("button", { name: "切换亮色主题" }).click();
  await expect(other.getByRole("button", { name: "切换暗色主题" })).toBeVisible();
  await expect(other.locator("html")).toHaveCSS("color-scheme", "light");
  await other.getByRole("button", { name: "切换暗色主题" }).click();
  await expect(page.getByRole("button", { name: "切换亮色主题" })).toBeVisible();
  await page.getByRole("button", { name: "切换亮色主题" }).click();
  await expect(other.getByRole("button", { name: "切换暗色主题" })).toBeVisible();
  await page.evaluate(() => localStorage.removeItem("sub2api-console-theme"));
  await expect(other.getByRole("button", { name: "切换亮色主题" })).toBeVisible();
  expect(await other.evaluate(() => localStorage.getItem("sub2api-console-theme"))).toBeNull();
  await other.close();
});

test("浏览器禁止本地存储时仍能登录前切换主题，刷新安全回退深色", async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "localStorage", {
      get: () => {
        throw new DOMException("禁止存储", "SecurityError");
      },
    });
  });
  await page.goto("/");
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "切换亮色主题" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect(page.locator('meta[name="theme-color"]')).toHaveAttribute("content", "#ffffff");
  await page.reload();
  await expect(page.getByRole("button", { name: "切换亮色主题" })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
});
