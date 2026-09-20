import { expect, test } from "@playwright/test";

import type { UpstreamConfiguration } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("编辑上游缓存详情后直接进入添加账号，准备完成后显示候选而无需刷新页面", async ({ page }) => {
  const configuration: UpstreamConfiguration = {
    upstream_id: "cached-entry",
    host: "cached.example.test",
    name: "缓存入口上游",
    base_url: "https://cached.example.test",
    account_base_url: "https://cached.example.test",
    upstream_type: "sub2api",
    auth_mode: "sub2api_user_token",
    recharge_rate: "1",
    raw_balance: "10",
    balance: "10",
    has_access_token: true,
    has_refresh_token: false,
    has_admin_key: false,
    has_user_id: false,
    headers: {},
    header_names: [],
    cookie_names: [],
    groups: [],
  };
  let allocation = {
    revision: "v1",
    target_id: configuration.upstream_id,
    upstream_id: configuration.upstream_id,
    override: null as boolean | null,
    selected: false,
    effective: false,
    global_enabled: true,
    source: "policy",
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === `/api/policy/upstream-concurrency/upstreams/${configuration.upstream_id}`) {
      if (route.request().method() === "PUT") {
        const payload = route.request().postDataJSON();
        expect(payload.expected_revision).toBe(allocation.revision);
        allocation = {
          ...allocation,
          revision: "v2",
          override: payload.override,
          selected: payload.override === true,
          effective: payload.override === true,
          source: "upstream",
        };
      }
      await route.fulfill({ json: allocation });
      return;
    }
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "加载回归" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/groups": [],
      "/api/upstreams/cached.example.test/configuration": configuration,
      "/api/upstreams": {
        hosts: [
          {
            ...configuration,
            hosts: [configuration.host],
            account_count: 0,
            group_count: 1,
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
      "/api/onboarding/prepare": {
        upstream: configuration,
        candidates: [
          {
            number: 1,
            upstream_id: configuration.upstream_id,
            host: configuration.host,
            upstream_name: configuration.name,
            group_id: "7",
            group_name: "准备完成的候选分组",
            description: null,
            platform: "openai",
            status: "active",
            multiplier: "1",
            recommended_binding: "",
            bindable: true,
            can_create_key: true,
            can_bind_existing_key: false,
            bound: false,
            key_present: false,
            upstream_key_id: null,
            upstream_key_name: null,
            recharge_rate: "1",
            unavailable_reason: null,
            bound_accounts: [],
          },
        ],
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (path in responses) {
      await route.fulfill({ json: responses[path] });
    } else if (path.startsWith("/api/dictionaries/")) {
      await route.fulfill({ json: { items: [] } });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/upstreams");
  await page.getByRole("button", { name: "编辑上游", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑上游", exact: true });
  await expect(dialog.getByRole("textbox", { name: "名称", exact: true })).toHaveValue(
    configuration.name,
  );
  const allocationSwitch = dialog.getByRole("switch", { name: "上游共享并发分配", exact: true });
  await expect(allocationSwitch).not.toBeChecked();
  await allocationSwitch.click();
  await expect(allocationSwitch).toBeChecked();
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  const [preparation] = await Promise.all([
    page.waitForResponse(
      (response) => new URL(response.url()).pathname === "/api/onboarding/prepare",
    ),
    page.getByRole("table").getByRole("button", { name: "添加账号", exact: true }).click(),
  ]);
  expect(preparation.ok()).toBe(true);
  await expect(page.getByRole("heading", { name: "账号添加", exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "当前上游概况" })).toBeVisible();
  await expect(page.getByText("准备完成的候选分组", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "正在获取", exact: true })).toHaveCount(0);
  await expect(page.getByRole("switch", { name: "上游共享并发分配", exact: true })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "本次新账号共享并发分配" })).toHaveCount(0);
  expect(allocation.override).toBeNull();
});
