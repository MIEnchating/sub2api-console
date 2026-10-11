import { expect, test, type Route } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("初始化等待冻结凭据，确认后不重复读取初始化状态", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const writes: Route[] = [];
  let reads = 0;
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/setup/status") {
      reads++;
      return route.fulfill({ json: { initialized: false, target_configured: true } });
    }
    if (path === "/api/setup/initialize") {
      writes.push(route);
      return;
    }
    if (path.endsWith("/automation/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/auth/session": { authenticated: true, username: "operator" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/overview": { open_alerts: 0 },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/events": [],
      "/api/dictionaries": { items: [] },
    };
    return route.fulfill({ json: fixtures[path] ?? {} });
  });
  await page.goto("/");
  await page.getByLabel("控制台账号", { exact: true }).fill("operator");
  await page.getByLabel("控制台密码", { exact: true }).fill("private-password");
  await page.getByLabel("确认控制台密码", { exact: true }).fill("private-password");
  await page.getByRole("button", { name: "完成初始化" }).click();
  await expect.poll(() => writes.length).toBe(1);
  await expect(page.getByLabel("控制台账号", { exact: true })).toBeDisabled();
  await expect(page.getByLabel("控制台密码", { exact: true })).toBeDisabled();
  await page.keyboard.press("Enter");
  expect(writes).toHaveLength(1);
  expect(writes[0].request().postDataJSON()).toEqual({
    username: "operator",
    password: "private-password",
    local_export_only: false,
    admin_base_url: "",
    admin_key: "",
  });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("setup-pending.png"), fullPage: true });
  await writes[0].fulfill({ json: { initialized: true, target_configured: true } });
  await expect(page.getByRole("heading", { name: "运营总览", exact: true })).toBeVisible();
  expect(reads).toBe(1);
});

test("初始化响应中断后读取到已完成状态，转入登录且不重放提交", async ({ page }) => {
  let initialized = false;
  let writes = 0;
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/setup/status")
      return route.fulfill({ json: { initialized, target_configured: false } });
    if (path === "/api/setup/initialize") {
      writes++;
      initialized = true;
      return route.abort("failed");
    }
    if (path === "/api/auth/session")
      return route.fulfill({ json: { authenticated: false, username: null } });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
  await page.goto("/");
  await page.getByLabel("控制台账号", { exact: true }).fill("operator");
  await page.getByLabel("控制台密码", { exact: true }).fill("private-password");
  await page.getByLabel("确认控制台密码", { exact: true }).fill("private-password");
  await page.getByRole("checkbox", { name: "仅使用本地账号工作台（不配置线上管理目标）" }).check();
  await page.getByRole("button", { name: "完成初始化" }).click();
  await expect(page.getByRole("heading", { name: "登录", exact: true })).toBeVisible();
  await expect(page.getByLabel("密码", { exact: true })).toHaveValue("");
  expect(writes).toBe(1);
});
