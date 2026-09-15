import { expect, test } from "@playwright/test";
import type { ManualAuthVerifyResult } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("恢复鉴权不再提供浏览器入口，填写 Token 后仍可验证保存", async ({ page }) => {
  const submissions: Record<string, unknown>[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const method = route.request().method();
    if (path === "/api/auth-recovery/manual" && method === "POST") {
      submissions.push(route.request().postDataJSON() as Record<string, unknown>);
      await route.fulfill({
        json: {
          host: "login.example.test",
          verified: true,
          balance_sync: { status: "succeeded", balance_status: "已同步" },
        } satisfies ManualAuthVerifyResult,
      });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "鉴权恢复测试" },
      "/api/auth-recovery/config": { auth_records: [], vault_entries: [] },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/upstreams": {
        hosts: [
          {
            upstream_id: "up_test",
            host: "login.example.test",
            hosts: ["login.example.test"],
            name: "测试上游",
            base_url: "https://login.example.test",
            upstream_type: "sub2api",
            auth_status: "鉴权失效",
            account_count: 0,
            group_count: 0,
            raw_balance: null,
            balance: null,
            display_balance: null,
            balance_unit: null,
            recharge_rate: "1",
            balance_status: "未读取",
            checked_at: null,
          },
        ],
        total_hosts: 1,
        authenticated_hosts: 0,
        recovery_required: 1,
        source: "test",
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/upstreams");
  await page.getByRole("button", { name: "更多操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "恢复鉴权", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "恢复鉴权", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("button", { name: /打开浏览器/ })).toHaveCount(0);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "验证并保存" })).toBeDisabled();
  await dialog.getByLabel("Token", { exact: true }).fill("isolated-access-token");
  await dialog.getByLabel("刷新 Token", { exact: true }).fill("isolated-refresh-token");
  await dialog.getByRole("button", { name: "验证并保存" }).click();
  await expect(dialog).toBeHidden();
  expect(submissions).toEqual([
    {
      host: "login.example.test",
      auth_mode: "sub2api_user_token",
      access_token: "isolated-access-token",
      refresh_token: "isolated-refresh-token",
    },
  ]);
  await expect(page.getByText(/凭证已验证并保存/)).toBeVisible();
});
