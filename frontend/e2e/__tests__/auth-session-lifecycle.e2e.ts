import { expect, test, type Route } from "@playwright/test";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "operator" },
      "/api/navigation-preferences": { hidden_item_ids: [], version: 1 },
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
    if (path.endsWith("/automation/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
});

test("退出等待禁止重复操作，失败保留页面，键盘重试后进入登录页", async ({ page }) => {
  const exits: Route[] = [];
  await page.route("**/api/auth/logout", (route) => {
    exits.push(route);
  });
  await page.goto("/");
  const exit = page.getByRole("button", { name: "退出登录", exact: true });
  await exit.click();
  await expect.poll(() => exits.length).toBe(1);
  await expect(exit).toBeDisabled();
  await expect(exit).toHaveAttribute("aria-busy", "true");
  await page.keyboard.press("Enter");
  expect(exits).toHaveLength(1);
  await exits[0].fulfill({ status: 503, json: { detail: "注销暂时失败，请重试" } });
  await expect(page.getByText("注销暂时失败，请重试", { exact: true })).toBeVisible();
  await expect(exit).toBeEnabled();
  await expect(page.getByRole("heading", { name: "运营总览", exact: true })).toBeVisible();
  await exit.focus();
  await exit.press("Enter");
  await expect.poll(() => exits.length).toBe(2);
  await exits[1].fulfill({ json: { authenticated: false, username: null } });
  await expect(page.getByRole("heading", { name: "登录", exact: true })).toBeVisible();
});

test("旧读取未完成时会话过期，重新登录无需再读会话且可继续刷新", async ({ page }) => {
  let sessionReads = 0;
  let expired = false;
  let oldRead: Route | undefined;
  let overviewReads = 0;
  await page.route("**/api/auth/session", async (route) => {
    sessionReads++;
    await route.fulfill({ json: { authenticated: true, username: "operator" } });
  });
  await page.route("**/api/overview", async (route) => {
    overviewReads++;
    if (overviewReads === 1) oldRead = route;
    else await route.fulfill({ json: { open_alerts: 0 } });
  });
  await page.route("**/api/accounts", (route) =>
    expired
      ? route.fulfill({ status: 401, json: { detail: "请重新登录" } })
      : route.fulfill({ json: [] }),
  );
  await page.route("**/api/auth/login", async (route) => {
    expired = false;
    await route.fulfill({ json: { authenticated: true, username: "operator" } });
  });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "运营总览", exact: true })).toBeVisible();
  await expect.poll(() => Boolean(oldRead)).toBe(true);
  expired = true;
  await page.getByRole("button", { name: "刷新运营总览" }).click();
  await expect(page.getByRole("alert")).toHaveText("登录已过期，请重新登录");
  await page.getByRole("textbox", { name: "账号", exact: true }).fill("operator");
  await page.getByLabel("密码", { exact: true }).fill("isolated-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "运营总览", exact: true })).toBeVisible();
  expect(sessionReads).toBe(1);
  await oldRead!.fulfill({ status: 401, json: { detail: "旧会话已过期" } });
  await page.getByRole("button", { name: "刷新运营总览" }).click();
  await expect(page.getByRole("button", { name: "刷新运营总览" })).toBeEnabled();
  await expect(page.getByRole("heading", { name: "运营总览", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "登录", exact: true })).toHaveCount(0);
  await page.screenshot({ path: test.info().outputPath("new-session-preserved.png") });
});
