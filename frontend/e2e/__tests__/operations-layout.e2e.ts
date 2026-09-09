import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { alerts, inspection, log, task } from "./fixtures/operations";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/inspection/automation": inspection,
      "/api/accounts": [],
      "/api/groups": [],
      "/api/tasks": [{ ...task, system_info: true }],
      "/api/tasks/layout-task": task,
      "/api/alerts": alerts,
      "/api/notifications/queue": {
        producer_firing: alerts,
        producer_recovered: [],
        consumer_pending: [],
        consumer_failed: [],
        consumer_items: [],
      },
      "/api/upstreams": {
        hosts: [],
        total_hosts: 0,
        authenticated_hosts: 0,
        recovery_required: 0,
        source: "test",
      },
      "/api/system/metrics": {
        cpu: { usage_percent: 20, logical_cores: 4 },
        memory: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
        disk: { usage_percent: 40, used_bytes: 1024, total_bytes: 4096 },
      },
      "/api/auth-recovery/config": {
        auth_records: [],
        vault_entries: [
          {
            entry: "巡检凭据",
            hosts: ["test.example.test"],
            has_username: true,
            has_password: true,
            username_is_email: true,
            header_names: [],
          },
        ],
      },
      "/api/logs": {
        items: [{ ...log, id: `event:${url.searchParams.get("page") ?? "1"}` }],
        total: 21,
        page: Number(url.searchParams.get("page") ?? 1),
        page_size: 20,
        counts: { event: 21 },
        truncated: false,
      },
      "/api/config/account-settings": {
        default: {
          models: [],
          concurrency: 10,
          load_factor: null,
          priority: 1,
          pool_mode: false,
          pool_mode_retry_count: 2,
          pool_mode_retry_status_codes: [429],
        },
        groups: [],
        platform_probe_models: {},
      },
    };
    if (url.pathname.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (url.pathname in fixtures) await route.fulfill({ json: fixtures[url.pathname] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

test("巡检队列和失败心跳有数据时可滚动打开详情并返回列表", async ({ page }) => {
  await page.goto("/auto-inspection");
  await page.getByRole("button", { name: "查看任务详情", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("主动探测");
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "查看心跳详情", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "巡检心跳详情" });
  await expect(dialog).toContainText("测试探测失败");
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("heading", { name: "自动巡检", exact: true })).toBeInViewport({
    ratio: 1,
  });
});

test("告警超过一页时分页可达，翻页展示剩余告警且标题保持可见", async ({ page }) => {
  await page.goto("/alerts");
  await expect(page.getByText(/巡检告警-1（/)).toBeVisible();
  await page.getByRole("button", { name: "转到下一页" }).click();
  await expect(page.getByText(/巡检告警-21（/)).toBeVisible();
  await expect(page.getByText(/巡检告警-1（/)).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "告警通知", exact: true })).toBeInViewport({
    ratio: 1,
  });
});

test("密码箱过滤后仍可编辑匹配凭据，关闭弹窗保留过滤条件", async ({ page }) => {
  await page.goto("/vault");
  const search = page.getByRole("textbox", { name: "搜索凭据" });
  await search.fill("巡检");
  await page.getByRole("button", { name: "编辑凭据", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "编辑凭据" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "凭据名称" })).toHaveValue("巡检凭据");
  await page.keyboard.press("Escape");
  await expect(search).toHaveValue("巡检");
  await expect(page.getByRole("heading", { name: "密码箱", exact: true })).toBeInViewport({
    ratio: 1,
  });
});

test("低高度系统信息中任务详情操作可达并完整展示任务进度", async ({ page, viewport }) => {
  await page.setViewportSize({ width: viewport!.width, height: 480 });
  await page.goto("/system-info");
  await page.getByRole("button", { name: "查看任务", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("正在读取测试上游");
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "刷新系统信息" })).toBeInViewport({ ratio: 1 });
});

test("日志有记录时列表独立滚动且翻页不会带走标题", async ({ page }) => {
  await page.goto("/logs");
  await expect(page.getByText("批量自动执行", { exact: true })).toBeVisible();
  const next = page.getByRole("button", { name: "转到下一页" });
  await next.click();
  await expect(page.getByRole("button", { name: "转到上一页" })).toBeEnabled();
  await expect(next).toBeDisabled();
  await expect(page.getByRole("heading", { name: "日志中心", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(
    await page
      .locator('[data-slot="page-content"]')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
});

for (const label of ["连接设置", "账号设置", "通知设置", "界面与日志"]) {
  test(`系统设置切换至${label}后底部操作可达且分类导航保持可见`, async ({ page }) => {
    await page.goto("/config");
    const tab = page.getByRole("tab", { name: label, exact: true });
    await tab.click();
    await expect(tab).toHaveAttribute("aria-selected", "true");
    const footer = page
      .locator('[data-testid="system-settings-panel"] [data-slot="settings-footer"]')
      .last();
    await footer.scrollIntoViewIfNeeded();
    await expect(footer).toBeInViewport({ ratio: 1 });
    await expect(page.getByRole("navigation", { name: "系统设置分类导航" })).toBeInViewport({
      ratio: 1,
    });
    expect(
      await page
        .locator('[data-slot="page-content"]')
        .evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
  });
}
