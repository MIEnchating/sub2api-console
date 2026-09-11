import { expect, test } from "@playwright/test";

import type { GroupStatus } from "../../src/api";
import { policy } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

const group: GroupStatus = {
  id: "6",
  name: "按钮尺寸回归测试的超长分组名称".repeat(4),
  account_count: 1,
  scheduling_open: 1,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "balanced",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "healthy",
  override: null,
};

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "按钮尺寸测试" },
      "/api/groups": [group],
      "/api/policy": policy,
      "/api/groups/6/probe-models": {
        group_id: "6",
        group_name: group.name,
        models: [],
        account_count: 1,
        accounts_with_models: 0,
        complete: true,
      },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("分组页的顶部、表格和批量操作按钮统一为 32px，长名称不会撑出窄屏", async ({ page }) => {
  await page.goto("/groups");
  const edit = page.getByRole("button", { name: "编辑分组", exact: true });
  await expect(edit).toBeEnabled();
  const commands = page
    .locator('[data-slot="page-heading"], [data-slot="table"]')
    .getByRole("button");
  expect(await commands.count()).toBeGreaterThan(2);
  for (const button of await commands.all()) {
    await expect(button).toHaveCSS("height", "32px");
    await expect(button).toHaveCSS("font-size", "14px");
  }
  await expect(edit).toHaveCSS("width", "32px");
  await page.getByRole("checkbox", { name: "选择当前页分组", exact: true }).click();
  const toolbar = page.getByRole("toolbar", { name: "已选择 1 个分组的批量操作" });
  await expect(toolbar).toBeInViewport({ ratio: 1 });
  for (const button of await toolbar.getByRole("button").all()) {
    await expect(button).toHaveCSS("height", "32px");
    await expect(button).toHaveCSS("width", "32px");
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("group-buttons.png") });
});

test("分组编辑窗口的策略选项、取消和保存按钮等高", async ({ page }) => {
  await page.goto("/groups");
  await page.getByRole("button", { name: "编辑分组", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑分组策略" });
  await expect(dialog).toBeVisible();
  for (const option of await dialog.getByRole("radio").all()) {
    await expect(option).toHaveCSS("height", "32px");
    await expect(option).toHaveCSS("font-size", "14px");
  }
  for (const name of ["取消", "保存策略"]) {
    const button = dialog.getByRole("button", { name, exact: true });
    await expect(button).toHaveCSS("height", "32px");
    await expect(button).toBeInViewport({ ratio: 1 });
  }
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).not.toBeVisible();
});

test("设置分类在键盘切换前后保持统一按钮高度", async ({ page }) => {
  await page.goto("/config");
  const tabs = page.getByRole("tablist", { name: "系统设置分类" });
  await expect(tabs).toBeVisible();
  for (const tab of await tabs.getByRole("tab").all()) {
    await expect(tab).toHaveCSS("height", "32px");
  }
  const connection = tabs.getByRole("tab", { name: "连接设置" });
  await connection.focus();
  await connection.press("End");
  const selected = tabs.getByRole("tab", { name: "界面与日志" });
  await expect(selected).toHaveAttribute("aria-selected", "true");
  await expect(selected).toBeFocused();
  await expect(selected).toHaveCSS("height", "32px");
});

test("未登录时提交按钮也使用统一高度并适应窄屏", async ({ page }) => {
  await page.route("**/api/auth/session", (route) =>
    route.fulfill({ json: { authenticated: false } }),
  );
  await page.goto("/");
  const login = page.getByRole("button", { name: "登录", exact: true });
  await expect(login).toHaveCSS("height", "32px");
  await expect(login).toBeInViewport({ ratio: 1 });
});
