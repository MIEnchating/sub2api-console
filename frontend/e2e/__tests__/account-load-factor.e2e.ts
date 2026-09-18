import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";

test("空负载账号回显跟随并发，键盘切换模式且保存不会固化有效值", async ({ page }, testInfo) => {
  const row = {
    ...account,
    id: "1213",
    name: "负载因子隔离账号",
    priority: 8500,
    load_factor: null,
    concurrency: 1,
    target_priority: null,
    target_load_factor: null,
    target_concurrency: null,
  };
  let submitted: Record<string, unknown> | null = null;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/accounts/1213/settings" && route.request().method() === "PUT") {
      submitted = route.request().postDataJSON() as Record<string, unknown>;
      await route.fulfill({ json: { id: "isolated-settings", status: "succeeded", result: {} } });
      return;
    }
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离测试" },
      "/api/accounts": [row],
      "/api/accounts/1213": {
        ...row,
        metadata: {},
        group_rates: {},
        group_ids: {},
        bindings: [],
        test_models: [],
      },
      "/api/model-checks/account-statuses": [],
      "/api/groups": [],
      "/api/dictionaries": { items: [] },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "isolated" },
      "/api/overview": { mode: "完全模式", account_count: 1, group_count: 0, open_alerts: 0 },
      "/api/policy": {
        available: true,
        advanced_policy: { manual_priority: { reserved_max: 10 } },
      },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: "retry: 86400000\n\n" });
    } else if (route.request().method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
    }
  });
  await page.goto("/accounts");
  const currentLoad = page.getByText("负载 1（跟随并发） · 并发 1", { exact: true });
  await currentLoad.scrollIntoViewIfNeeded();
  await expect(currentLoad).toBeVisible();
  await page.getByRole("button", { name: "更多账号操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "查看并编辑账号", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "账号设置", exact: true });
  const load = dialog.getByRole("spinbutton", { name: "负载因子", exact: true });
  const follow = dialog.getByRole("switch", { name: "跟随并发上限" });
  await expect(follow).toBeChecked();
  await expect(load).toHaveValue("1");
  await expect(load).toBeDisabled();
  await follow.focus();
  await page.keyboard.press("Space");
  await expect(follow).not.toBeChecked();
  await expect(load).toBeEnabled();
  await load.fill("7");
  await follow.focus();
  await page.keyboard.press("Space");
  await expect(follow).toBeChecked();
  await dialog.getByRole("spinbutton", { name: "并发上限", exact: true }).fill("5");
  await expect(load).toHaveValue("5");
  await page.screenshot({ path: testInfo.outputPath("load-factor-settings.png") });
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect
    .poll(() => submitted)
    .toMatchObject({ follow_concurrency: true, load_factor: "", concurrency: 5 });
  await expect(dialog).toHaveCount(0);
});
