import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "设置布局测试" },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "test" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi": {
        platforms: [],
        local_groups: [],
        bindings: [],
        sub2api_base_url: "https://sub.example.test",
      },
      "/api/uptime-kuma/config": {
        base_url: "",
        username: "",
        api_key_configured: false,
        management_configured: false,
        revision: 0,
      },
    };
    if (route.request().method() !== "GET")
      return route.fulfill({ status: 503, json: { detail: "隔离测试不执行写入" } });
    if (path.endsWith("/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    if (path in fixtures) return route.fulfill({ json: fixtures[path] });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
});

test("旧平台入口跳转系统设置，字段说明可读且移动端表单不溢出", async ({ page }) => {
  await page.goto("/newapi");
  await expect(page).toHaveURL(/\/config\?tab=newapi$/);
  await expect(page.getByRole("heading", { level: 1, name: "系统设置" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "New API 平台" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByRole("link", { name: "平台配置", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "接入配置", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "添加平台配置", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "添加 New API 平台配置" });
  await expect(dialog.getByLabel("平台名称", { exact: true })).toHaveAttribute(
    "aria-required",
    "true",
  );
  await expect(dialog.getByLabel("Admin Key", { exact: true })).toHaveAttribute(
    "aria-required",
    "true",
  );
  await expect(dialog.getByText(/首次接入或更换地址必填/)).toBeVisible();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("newapi-fields.png") });
  await dialog.getByRole("button", { name: "取消" }).click();
  await page.getByRole("tab", { name: "监控平台" }).click();
  await expect(page).toHaveURL(/\/config\?tab=monitoring$/);
  await expect(page.getByLabel("服务地址", { exact: true })).toHaveAttribute(
    "aria-required",
    "true",
  );
  await expect(page.getByLabel("管理账号（可选）", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "验证并保存", exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("monitoring-settings.png") });
  const otp = page.getByLabel("两步验证码（已启用时填写）", { exact: true });
  await otp.scrollIntoViewIfNeeded();
  await expect(otp).toBeInViewport();
  await page.goto("/newapi/channels");
  await page.getByRole("button", { name: "前往系统设置配置平台" }).click();
  await expect(page).toHaveURL(/\/config\?tab=newapi$/);
  await page.goto("/uptime-kuma/config");
  await expect(page).toHaveURL(/\/config\?tab=monitoring$/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
});

test("切换有无刷新按钮的设置分类时，顶部页签位置和宽度保持不变", async ({ page }) => {
  await page.goto("/config?tab=newapi");
  const tabs = page.getByRole("navigation", { name: "系统设置分类导航" });
  await expect(tabs).toBeVisible();
  const initial = await tabs.boundingBox();
  expect(initial).not.toBeNull();
  for (const name of ["连接设置", "监控平台", "通知设置", "New API 平台"]) {
    const tab = page.getByRole("tab", { name, exact: true });
    await tab.click();
    await expect(tab).toHaveAttribute("aria-selected", "true");
    await expect
      .poll(async () => {
        const box = await tabs.boundingBox();
        return { x: box?.x, y: box?.y, width: box?.width, height: box?.height };
      })
      .toEqual(initial);
  }
});
