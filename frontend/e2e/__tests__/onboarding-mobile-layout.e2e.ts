import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

const upstream = {
  upstream_id: "mobile-layout",
  host: "mobile.example.test",
  name: "移动端布局测试上游",
  base_url: "https://mobile.example.test",
  account_base_url: "https://mobile.example.test",
  upstream_type: "sub2api",
  auth_mode: "sub2api_user_token",
  recharge_rate: "1",
  balance: "438.99217628",
  groups: [],
  headers: {},
  header_names: [],
  cookie_names: [],
};

const candidate = {
  number: 1,
  host: upstream.host,
  upstream_id: upstream.upstream_id,
  upstream_name: upstream.name,
  group_id: "1",
  group_name: "移动分组 1",
  platform: "openai",
  status: "active",
  multiplier: "1",
  description: "移动端分组说明".repeat(20),
  bindable: true,
  can_create_key: true,
  can_bind_existing_key: false,
  bound: false,
  key_present: false,
  bound_accounts: [],
  unavailable_reason: null,
};

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/policy/upstream-concurrency/upstreams/mobile-layout": {
        revision: "v1",
        target_id: upstream.upstream_id,
        upstream_id: upstream.upstream_id,
        override: null,
        selected: true,
        effective: true,
        global_enabled: true,
        source: "policy",
      },
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "移动端布局回归" },
      "/api/dictionaries": { items: [] },
      "/api/upstreams/mobile.example.test/configuration": upstream,
      "/api/groups": [{ id: "3", name: "本地 OpenAI", platform: "openai", account_count: 0 }],
      "/api/onboarding/prepare": {
        upstream,
        candidates: Array.from({ length: 21 }, (_, index) => ({
          ...candidate,
          number: index + 1,
          group_id: String(index + 1),
          group_name: `移动分组 ${index + 1}`,
        })),
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

for (const viewport of [
  { width: 320, height: 568 },
  { width: 390, height: 664 },
  { width: 640, height: 480 },
  { width: 1440, height: 900 },
]) {
  test(`${viewport.width}px 账号添加工具栏换行后分组仍可查看、滚动和翻页`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/onboarding?host=mobile.example.test&upstream_type=sub2api");
    const container = page.locator('[data-slot="table-container"]');
    const first = page.getByText("移动分组 1", { exact: true });
    await expect(first).toBeVisible();
    await expect(page.getByRole("region", { name: "并发分配设置", exact: true })).toHaveCount(0);
    await expect(page.getByRole("switch", { name: "上游共享并发分配" })).toHaveCount(0);
    await expect(page.getByRole("combobox", { name: "本次新账号共享并发分配" })).toHaveCount(0);
    // 表头和至少一条完整分组行必须有可用高度，不能只剩分页栏。
    await expect
      .poll(() => container.evaluate((element) => element.clientHeight))
      .toBeGreaterThanOrEqual(160);
    await first.scrollIntoViewIfNeeded();
    await expect(first).toBeInViewport({ ratio: 1 });
    await page.screenshot({ path: test.info().outputPath("onboarding-groups.png") });
    const last = page.getByText("移动分组 20", { exact: true });
    await last.scrollIntoViewIfNeeded();
    await expect(last).toBeInViewport({ ratio: 1 });
    await page.getByRole("button", { name: "转到下一页" }).click();
    const next = page.getByText("移动分组 21", { exact: true });
    await container.scrollIntoViewIfNeeded();
    await next.scrollIntoViewIfNeeded();
    await expect(next).toBeInViewport({ ratio: 1 });
    const preview = page.getByRole("button", { name: "预览 0 项变更" });
    await preview.scrollIntoViewIfNeeded();
    await expect(preview).toBeInViewport({ ratio: 1 });
    await expect(preview).toBeDisabled();
    const content = page.locator('[data-slot="page-content"]');
    expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
  });
}

test("窄屏只有一条已绑定分组时仍显示分组、状态及禁用的预览入口", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 664 });
  await page.route("**/api/onboarding/prepare", (route) =>
    route.fulfill({
      json: {
        upstream,
        candidates: [
          {
            ...candidate,
            bound: true,
            can_create_key: false,
            bound_accounts: [
              {
                binding_id: 1,
                account_id: "9",
                account_name: "已绑定账号",
                base_url: upstream.base_url,
                account_exists: true,
                binding_status: "active",
                local_group: "本地 OpenAI",
                local_groups: [{ id: "3", name: "本地 OpenAI" }],
                upstream_key_id: "7",
                upstream_key_name: "绑定 Key",
              },
            ],
          },
        ],
      },
    }),
  );
  await page.goto("/onboarding?host=mobile.example.test&upstream_type=sub2api");
  const row = page.getByRole("row").filter({ hasText: "移动分组 1" });
  await row.getByText("移动分组 1", { exact: true }).scrollIntoViewIfNeeded();
  await expect(row.getByText("已绑定", { exact: true })).toBeInViewport({ ratio: 1 });
  const preview = page.getByRole("button", { name: "预览 0 项变更" });
  await preview.scrollIntoViewIfNeeded();
  await expect(preview).toBeInViewport({ ratio: 1 });
  await expect(preview).toBeDisabled();
});

test("窄屏搜索无匹配分组时空提示可见，清空搜索后恢复分组列表", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await page.goto("/onboarding?host=mobile.example.test&upstream_type=sub2api");
  const search = page.getByRole("searchbox", { name: "搜索上游分组" });
  await search.fill("不存在的分组");
  const empty = page.getByText("没有匹配的上游分组", { exact: true });
  await empty.scrollIntoViewIfNeeded();
  await expect(empty).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("navigation", { name: "表格分页" })).toHaveCount(0);
  await search.fill("");
  const first = page.getByText("移动分组 1", { exact: true });
  await first.scrollIntoViewIfNeeded();
  await expect(first).toBeInViewport({ ratio: 1 });
});

test("窄屏加载上游时骨架列表保留高度，响应完成后显示真实分组", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 568 });
  let release = (): void => {};
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/onboarding/prepare", async (route) => {
    await pending;
    await route.fulfill({ json: { upstream, candidates: [candidate] } });
  });
  await page.goto("/onboarding?host=mobile.example.test&upstream_type=sub2api");
  try {
    const loading = page.getByRole("status", { name: "正在获取上游信息" });
    await expect(loading).toBeVisible();
    const container = loading.locator('[data-slot="table-container"]');
    await expect
      .poll(() => container.evaluate((element) => element.clientHeight))
      .toBeGreaterThanOrEqual(160);
  } finally {
    release();
  }
  await expect(page.getByText("移动分组 1", { exact: true })).toBeVisible();
});

test("窄屏横向滚动到本地分组后可用键盘选择，批量预览同步启用", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 664 });
  await page.goto("/onboarding?host=mobile.example.test&upstream_type=sub2api");
  const binding = page.getByRole("combobox", { name: "移动分组 1 本地分组" });
  await binding.focus();
  await binding.press("Enter");
  await page.getByRole("option", { name: /本地 OpenAI/ }).click();
  await page.keyboard.press("Escape");
  const preview = page.getByRole("button", { name: "预览 1 项变更" });
  await preview.scrollIntoViewIfNeeded();
  await expect(preview).toBeInViewport({ ratio: 1 });
  await expect(preview).toBeEnabled();
});
