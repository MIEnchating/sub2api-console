import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

const channels = Array.from({ length: 20 }, (_, index) => ({
  id: String(index + 1),
  name: index === 0 ? "标准渠道 / 多模型业务" : `渠道 ${index + 1}`,
  type: 59,
  status: index === 3 ? 2 : 1,
  groups: [index % 2 ? "vip" : "default"],
  models: ["gpt-5", "gpt-5-mini", "claude-sonnet-4-6"],
  version: "a".repeat(64),
}));

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
      "/api/auth/session": { authenticated: true, username: "渠道布局测试" },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "test" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/channels": { items: channels, total: channels.length },
      "/api/newapi/platforms/layout/channel-groups": { groups: [], version: "" },
      "/api/newapi/platforms/layout/refresh": {
        groups: [],
        models: [],
        unset_models: [],
        references: [],
        tool_prices: [],
        differences: [],
      },
      "/api/auth-recovery/config": { vault_entries: [] },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path.startsWith("/api/dictionaries")) await route.fulfill({ json: { items: [] } });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离布局测试未配置接口" } });
  });
});

test("渠道表格独立滚动，多选操作与新增弹窗适配桌面和手机", async ({ page }) => {
  await page.goto("/newapi/channels");
  const table = page.getByRole("table", { name: "现有渠道" });
  await expect(table).toBeVisible();
  const container = table.locator("..");
  await expect(container).toHaveCSS("overflow-y", "auto");
  const pagination = page.getByRole("navigation", { name: "表格分页" });
  await expect(pagination).toBeVisible();
  await expect(pagination.getByRole("combobox", { name: "每页行数" })).toBeVisible();
  await expect(pagination.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  expect(await pagination.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("channels.png") });
  await page.getByRole("checkbox", { name: "全选当前筛选渠道" }).check();
  const toolbar = page.getByRole("toolbar", { name: "已选择 20 个渠道的批量操作" });
  await expect(toolbar).toBeVisible();
  await toolbar.getByRole("button", { name: "批量下架模型" }).click();
  const batch = page.getByRole("dialog", { name: /下架模型/ });
  await batch.getByRole("checkbox", { name: "gpt-5-mini", exact: true }).check();
  await batch.getByRole("button", { name: "预览变更" }).click();
  await expect(batch.getByRole("group", { name: "渠道影响范围" })).toBeVisible();
  expect(await batch.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("channel-batch-preview.png") });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "清空选择" }).click();
  await page.getByRole("button", { name: "新增渠道" }).click();
  const create = page.getByRole("dialog", { name: "新增渠道" });
  await expect(create.getByRole("button", { name: "自定义账号密码" })).toBeVisible();
  expect(await create.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("channel-create.png") });
  await page.keyboard.press("Escape");
  await expect(create).not.toBeVisible();
});
