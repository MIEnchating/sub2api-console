import { expect, test } from "@playwright/test";
import type { UpstreamConfiguration } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("上游保存中可关闭重开，迟到响应保留新草稿且关闭后焦点返回入口", async ({ page }) => {
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  let writes = 0;
  const configuration: UpstreamConfiguration = {
    upstream_id: "up_editor",
    host: "editor.example.test",
    name: "编辑测试上游",
    base_url: "https://editor.example.test",
    account_base_url: "https://editor.example.test/v1",
    upstream_type: "sub2api",
    auth_mode: "sub2api_user_token",
    recharge_rate: "1",
    raw_balance: "24",
    balance: "24",
    has_access_token: true,
    has_refresh_token: false,
    has_admin_key: false,
    has_user_id: false,
    headers: {},
    header_names: [],
    cookie_names: [],
    groups: [],
  };
  const configPath = "/api/upstreams/editor.example.test/configuration";
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === configPath && route.request().method() === "PUT") {
      writes++;
      await pending;
      await route.fulfill({ json: { ...configuration, name: "已保存的旧名称", balance: "73" } });
      return;
    }
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离上游编辑测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/policy/upstream-concurrency/upstreams/up_editor": {
        revision: "v1",
        target_id: "up_editor",
        upstream_id: "up_editor",
        override: null,
        selected: true,
        effective: true,
        global_enabled: true,
        source: "policy",
      },
      [configPath]: configuration,
      "/api/upstreams": {
        hosts: [
          {
            ...configuration,
            hosts: [configuration.host],
            account_count: 0,
            group_count: 0,
            auth_status: "已鉴权",
            balance_status: "已读取",
            checked_at: null,
          },
        ],
        total_hosts: 1,
        authenticated_hosts: 1,
        recovery_required: 0,
        source: "test",
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in responses && route.request().method() === "GET")
      await route.fulfill({ json: responses[path] });
    else if (path.startsWith("/api/dictionaries/")) await route.fulfill({ json: { items: [] } });
    else await route.fulfill({ status: 503, json: { detail: "未配置的隔离接口" } });
  });
  await page.goto("/upstreams");
  const trigger = page.getByRole("button", { name: "编辑上游", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "编辑上游", exact: true });
  await dialog.getByRole("textbox", { name: "名称", exact: true }).fill("第一次编辑");
  await dialog.getByRole("button", { name: "保存并重算" }).click();
  await expect.poll(() => writes).toBe(1);
  await expect(dialog.getByRole("textbox", { name: "名称", exact: true })).toBeDisabled();
  await expect(dialog.getByRole("combobox", { name: "平台", exact: true })).toBeDisabled();
  await expect(dialog.getByRole("tab", { name: "关联账号" })).toBeDisabled();
  await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeEnabled();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await trigger.click();
  await dialog.getByRole("textbox", { name: "名称", exact: true }).fill("重新打开的草稿");
  const response = page.waitForResponse(
    (value) => new URL(value.url()).pathname === configPath && value.request().method() === "PUT",
  );
  release();
  await response;
  await expect(dialog.getByLabel("换算后余额", { exact: true })).toHaveText("73");
  await expect(dialog.getByRole("textbox", { name: "名称", exact: true })).toHaveValue(
    "重新打开的草稿",
  );
  await expect(dialog.getByRole("button", { name: "保存并重算" })).toBeEnabled();
  await expect(page.getByText("上游配置已保存", { exact: true })).toHaveCount(0);
  expect(writes).toBe(1);
});
