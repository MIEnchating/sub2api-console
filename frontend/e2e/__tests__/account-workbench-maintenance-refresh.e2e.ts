import { expect, test, type Page, type Route } from "@playwright/test";
import type { WorkbenchMaintenance } from "../../src/features/account-workbench/types";
import { template } from "../../src/features/account-workbench/__tests__/template-fixture";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

async function setup(page: Page): Promise<{ value: WorkbenchMaintenance; writes: string[] }> {
  const state = {
    value: {
      revision: 3,
      enabled: false,
      interval_minutes: 5,
      cooldown_minutes: 10,
      check_after_repair: true,
      group_ids: [],
      running: false,
      task_id: "",
      last_check_at: null,
      next_check_at: null,
      message: "",
      results: [],
    } as WorkbenchMaintenance,
    writes: [] as string[],
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() !== "GET") {
      state.writes.push(path);
      if (path.endsWith("/maintenance/preview")) {
        await route.fulfill({
          json: {
            id: "preview-one",
            revision: 1,
            expires_at: "2099-01-01T00:00:00Z",
            settings: route.request().postDataJSON(),
            accounts: [template.summary],
          },
        });
      } else await route.fulfill({ status: 503, json: { detail: "隔离测试禁止写入" } });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "维护刷新测试" },
      "/api/dictionaries": { items: [] },
      "/api/account-workbench/accounts": [template.summary],
      "/api/account-workbench/maintenance": state.value,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  return state;
}

test("维护设置刷新同步未编辑字段，编辑冲突保留草稿且可键盘载入最新值", async ({ page }) => {
  await page.clock.install();
  const state = await setup(page);
  await page.goto("/account-workbench?tab=maintenance");
  await page.getByRole("button", { name: "维护设置", exact: true }).click();
  const interval = page.getByRole("spinbutton", { name: "检查间隔（分钟）" });
  await expect(interval).toHaveValue("5");
  state.value = {
    ...state.value,
    revision: 4,
    interval_minutes: 20,
    enabled: true,
    group_ids: ["7"],
  };
  await page.clock.fastForward(30_000);
  await expect(interval).toHaveValue("20");
  await expect(page.getByRole("combobox", { name: "维护分组" })).toContainText("团队");
  await interval.fill("12");
  state.value = { ...state.value, revision: 5, interval_minutes: 30, cooldown_minutes: 40 };
  await page.clock.fastForward(30_000);
  const reload = page.getByRole("button", { name: "载入最新设置" });
  await expect(reload).toBeEnabled();
  await expect(interval).toHaveValue("12");
  await expect(page.getByRole("spinbutton", { name: "失败冷却（分钟）" })).toHaveValue("10");
  await expect(page.getByRole("button", { name: "预览并保存" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "立即检查", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "停止维护" })).toBeEnabled();
  await expect(
    page.locator("[data-sonner-toast]").filter({ hasText: "维护设置已更新" }),
  ).toHaveCount(1);
  await reload.scrollIntoViewIfNeeded();
  await expect(reload).toBeInViewport({ ratio: 1 });
  expect(
    await page
      .getByRole("dialog", { name: "维护设置", exact: true })
      .evaluate((node) => node.scrollWidth <= node.clientWidth),
  ).toBe(true);
  await page.screenshot({ path: test.info().outputPath("maintenance-conflict.png") });
  await reload.focus();
  await page.keyboard.press("Enter");
  await expect(interval).toHaveValue("30");
  await expect(page.getByRole("spinbutton", { name: "失败冷却（分钟）" })).toHaveValue("40");
  await expect(page.getByRole("button", { name: "预览并保存" })).toBeEnabled();
  await expect(reload).toHaveCount(0);
  expect(state.writes).toEqual([]);
});

test("维护分组失败提供重试，恢复后保留编辑值并可选择范围", async ({ page }) => {
  await page.clock.install();
  const state = await setup(page);
  const held: Route[] = [];
  let hold = true;
  await page.route("**/api/account-workbench/accounts", async (route) => {
    if (hold) held.push(route);
    else await route.fulfill({ json: [template.summary] });
  });
  await page.goto("/account-workbench?tab=maintenance");
  await page.getByRole("button", { name: "维护设置", exact: true }).click();
  await expect(page.getByRole("status", { name: "正在读取维护分组" })).toBeVisible();
  const selector = page.getByRole("combobox", { name: "维护分组" });
  await expect(selector).toHaveAttribute("aria-disabled", "true");
  await expect.poll(() => held.length).toBeGreaterThan(0);
  for (const route of held)
    await route.fulfill({ status: 422, json: { detail: "账号目录暂不可用" } });
  const retry = page.getByRole("button", { name: "重新读取", exact: true });
  await expect(retry).toBeEnabled();
  await expect(selector).toHaveAttribute("aria-disabled", "true");
  const interval = page.getByRole("spinbutton", { name: "检查间隔（分钟）" });
  await interval.fill("12");
  hold = false;
  await retry.click();
  await expect(selector).not.toHaveAttribute("aria-disabled", "true");
  await expect(interval).toHaveValue("12");
  await selector.click();
  await page.getByRole("option", { name: "团队", exact: true }).click();
  await expect(selector).toContainText("团队");
  expect(state.writes).toEqual([]);
});

test("维护预览等待期间后台配置变化阻止确认旧范围且允许返回", async ({ page }) => {
  await page.clock.install();
  const state = await setup(page);
  let held: Route | undefined;
  await page.route("**/api/account-workbench/maintenance/preview", (route) => {
    held = route;
  });
  await page.goto("/account-workbench?tab=maintenance");
  await page.getByRole("button", { name: "维护设置", exact: true }).click();
  await expect(page.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toHaveValue("5");
  await page.getByRole("button", { name: "预览并保存" }).click();
  await expect.poll(() => !!held).toBe(true);
  state.value = { ...state.value, revision: 4, interval_minutes: 20 };
  await page.clock.fastForward(30_000);
  await expect(page.getByRole("button", { name: "载入最新设置" })).toBeDisabled();
  await held!.fulfill({
    json: {
      id: "preview-one",
      revision: 1,
      expires_at: "2099-01-01T00:00:00Z",
      settings: held!.request().postDataJSON(),
      accounts: [template.summary],
    },
  });
  const dialog = page.getByRole("dialog", { name: "确认维护范围 · 1 个账号", exact: true });
  await expect(dialog.getByRole("button", { name: "确认保存" })).toBeDisabled();
  await dialog.getByRole("button", { name: "返回修改" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("dialog", { name: "维护设置", exact: true })).toBeVisible();
  await expect(page.getByRole("spinbutton", { name: "检查间隔（分钟）" })).toHaveValue("20");
  expect(state.writes).toEqual([]);
});
