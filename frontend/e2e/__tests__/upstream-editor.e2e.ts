import { expect, test } from "@playwright/test";

import type { UpstreamConfiguration } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("编辑上游在桌面和手机上展示并发，长内容不会挤出页脚和账号操作", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  const configuration: UpstreamConfiguration = {
    upstream_id: "up_editor",
    host: "editor.example.test",
    name: "共享并发上游",
    base_url: `https://editor.example.test/${"very-long-path/".repeat(12)}`,
    account_base_url: "https://models.example.test/v1",
    upstream_type: "sub2api",
    auth_mode: "sub2api_user_token",
    recharge_rate: "1",
    raw_balance: "123456789012345678901234567890.12",
    balance: "123456789012345678901234567890.12",
    concurrency_limit: 1000,
    concurrency_status: "known",
    allocated_concurrency: 800,
    target_concurrency: 800,
    has_access_token: true,
    has_refresh_token: true,
    has_admin_key: false,
    has_user_id: false,
    headers: {},
    header_names: [],
    cookie_names: [],
    groups: [
      {
        upstream_id: "up_editor",
        host: "editor.example.test",
        group_id: "7",
        name: "很长的上游模型分组名称".repeat(5),
        description: null,
        platform: "openai",
        status: "active",
        raw_rate: "1",
        effective_rate: "1",
        recharge_rate: "1",
        bound: true,
        key_present: true,
        bindable: true,
        unavailable_reason: null,
        bound_accounts: [
          {
            binding_id: 1,
            account_id: "41",
            account_name: "很长的关联账号名称".repeat(8),
            account_exists: true,
            binding_status: "active",
            local_group: "测试分组",
            upstream_key_id: "key-41",
            upstream_key_name: "测试Key",
          },
        ],
      },
    ],
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "编辑弹窗测试" },
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
      "/api/upstreams/editor.example.test/configuration": configuration,
      "/api/upstreams": {
        hosts: [
          {
            ...configuration,
            hosts: [configuration.host],
            account_count: 1,
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
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in responses) {
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
  await expect(dialog.getByLabel("已配置并发", { exact: true })).toHaveText("800");
  await expect(dialog.getByLabel("用户上限", { exact: true })).toHaveText("1000");
  await expect(dialog.getByLabel("目标并发", { exact: true })).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "保存并重算" })).toBeInViewport();
  await expect(dialog.getByRole("heading", { name: "编辑上游", exact: true })).toBeFocused();
  await expect(dialog.getByRole("switch", { name: "上游共享并发分配" })).toBeChecked();
  await dialog.evaluate(async (element) => {
    await Promise.all(element.getAnimations().map((animation) => animation.finished));
  });
  const alignment = await dialog.evaluate((element) => {
    const summary = element.querySelector('[aria-labelledby="upstream-concurrency-title"]')!;
    const allocation = element.querySelector('section[aria-label="上游共享并发分配"]')!;
    const control = allocation.querySelector('[role="switch"]')!;
    const left = summary.getBoundingClientRect();
    const right = allocation.getBoundingClientRect();
    const toggle = control.getBoundingClientRect();
    return {
      leftCenter: left.top + left.height / 2,
      rightCenter: right.top + right.height / 2,
      toggleCenter: toggle.top + toggle.height / 2,
      leftBottom: left.bottom,
      rightTop: right.top,
    };
  });
  if (page.viewportSize()!.width >= 1024) {
    expect(alignment.rightCenter).toBeCloseTo(alignment.leftCenter, 0);
    expect(alignment.toggleCenter).toBeCloseTo(alignment.leftCenter, 0);
  } else {
    expect(alignment.rightTop).toBeGreaterThanOrEqual(alignment.leftBottom);
  }
  const body = dialog.locator('[data-slot="dialog-body"]');
  expect(await body.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const connection = await dialog.getByRole("region", { name: "连接设置" }).boundingBox();
  const authentication = await dialog.getByRole("region", { name: "鉴权配置" }).boundingBox();
  expect(connection).not.toBeNull();
  expect(authentication).not.toBeNull();
  if (page.viewportSize()!.width >= 1024) {
    expect(authentication!.x).toBeGreaterThanOrEqual(connection!.x + connection!.width);
    expect(authentication!.y).toBe(connection!.y);
  } else {
    expect(authentication!.y).toBeGreaterThanOrEqual(connection!.y + connection!.height);
  }
  const saveBox = await dialog.getByRole("button", { name: "保存并重算" }).boundingBox();
  const cancelBox = await dialog.getByRole("button", { name: "取消", exact: true }).boundingBox();
  expect(saveBox!.height).toBe(32);
  expect(cancelBox!.y).toBe(saveBox!.y);
  await expect(dialog.getByRole("switch", { name: "上游共享并发分配" })).toBeChecked();
  await page.screenshot({ path: `/tmp/sub2api-upstream-editor-${testInfo.project.name}.png` });
  await dialog.getByRole("tab", { name: "关联账号", exact: true }).click();
  await dialog.getByRole("region", { name: "当前上游账号" }).scrollIntoViewIfNeeded();
  await expect(dialog.getByRole("button", { name: "探活测试", exact: true })).toBeInViewport();
  await expect(
    dialog.getByRole("button", { name: "删除账号及上游 Key", exact: true }),
  ).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "保存并重算" })).toHaveCount(0);
  await page.screenshot({
    path: `/tmp/sub2api-upstream-editor-accounts-${testInfo.project.name}.png`,
  });
  expect(await body.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});
