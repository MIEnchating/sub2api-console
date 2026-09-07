import { expect, test } from "@playwright/test";

for (const failure of ["access", "write"] as const) {
  test(`浏览器存储${failure === "access" ? "禁止访问" : "写入失败"}时仍可进入登录页面`, async ({
    page,
  }) => {
    await page.addInitScript((mode) => {
      if (mode === "access") {
        Object.defineProperty(window, "localStorage", {
          get() {
            throw new DOMException("Storage unavailable", "SecurityError");
          },
        });
      } else {
        Storage.prototype.setItem = () => {
          throw new DOMException("Storage full", "QuotaExceededError");
        };
      }
    }, failure);
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/setup/status") {
        await route.fulfill({ json: { initialized: true, configuration_errors: [] } });
      } else if (path === "/api/auth/session") {
        await route.fulfill({ json: { authenticated: false, username: null } });
      } else {
        await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      }
    });
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "登录", exact: true })).toBeVisible();
    await expect(page.getByLabel("账号", { exact: true })).toBeEditable();
  });
}
