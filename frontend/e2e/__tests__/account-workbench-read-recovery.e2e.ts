import { expect, test, type Page, type Route } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { template } from "../../src/features/account-workbench/__tests__/template-fixture";

async function setup(page: Page): Promise<void> {
  const fixtures: Record<string, unknown> = {
    ...pageFixtures,
    "/api/setup/status": { initialized: true, configuration_errors: [] },
    "/api/auth/session": { authenticated: true, username: "工作台读取测试" },
    "/api/dictionaries": { items: [] },
    "/api/account-workbench/templates": { revision: 1, preferred_id: "", items: [template] },
    "/api/account-workbench/accounts": [template.summary],
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
}

test("模板加载可继续编辑，读取失败可键盘重试且不丢失账号资料", async ({ page }) => {
  await setup(page);
  let held: Route | undefined;
  let failed = false;
  await page.route("**/api/account-workbench/templates", async (route) => {
    if (failed) await route.fulfill({ status: 422, json: { detail: "模板暂不可用" } });
    else held = route;
  });
  await page.goto("/account-workbench");
  const loading = page.getByRole("status", { name: "正在读取配置模板" });
  await expect(loading).toBeVisible();
  const input = page.getByRole("textbox", { name: "账号资料", exact: true });
  await input.fill("rt_isolated");
  const selector = page.getByRole("combobox", { name: "配置模板", exact: true });
  await expect(selector).toBeDisabled();
  await expect(page.getByRole("button", { name: "解析并预览" })).toBeEnabled();
  await expect.poll(() => !!held).toBe(true);
  failed = true;
  await held!.fulfill({ status: 422, json: { detail: "模板暂不可用" } });
  const retry = page.getByRole("button", { name: "重新读取", exact: true });
  await expect(retry).toBeEnabled();
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: "模板暂不可用" })).toHaveCount(
    1,
  );
  await expect(selector).toBeDisabled();
  await expect(input).toHaveValue("rt_isolated");
  await retry.scrollIntoViewIfNeeded();
  await expect(retry).toBeInViewport({ ratio: 1 });
  expect(
    await page.getByRole("tabpanel").evaluate((node) => node.scrollWidth <= node.clientWidth),
  ).toBe(true);
  await page.screenshot({ path: test.info().outputPath("template-retry.png") });
  failed = false;
  held = undefined;
  await retry.focus();
  await page.keyboard.press("Enter");
  await expect(loading).toBeVisible();
  await expect.poll(() => !!held).toBe(true);
  await held!.fulfill({ json: { revision: 1, preferred_id: "", items: [template] } });
  await expect(selector).toBeEnabled();
  await expect(retry).toHaveCount(0);
  await selector.click();
  await page.getByRole("option", { name: template.name, exact: true }).click();
  await expect(selector).toContainText(template.name);
  await expect(input).toHaveValue("rt_isolated");
});

test("独立输出隐藏模板等待，切回导入后重新读取并保留输入", async ({ page }) => {
  await setup(page);
  const held: Route[] = [];
  await page.route("**/api/account-workbench/templates", (route) => {
    held.push(route);
  });
  await page.goto("/account-workbench");
  await expect(page.getByRole("status", { name: "正在读取配置模板" })).toBeVisible();
  const input = page.getByRole("textbox", { name: "账号资料", exact: true });
  await input.fill("rt_isolated");
  await expect.poll(() => held.length).toBeGreaterThan(0);
  await page.getByRole("combobox", { name: "处理方式" }).click();
  await page.getByRole("option", { name: "独立 JSON 输出" }).click();
  await expect(page.getByRole("status", { name: "正在读取配置模板" })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "配置模板" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "解析并预览" })).toBeEnabled();
  const previousReads = held.length;
  await page.getByRole("combobox", { name: "处理方式" }).click();
  await page.getByRole("option", { name: "导入目标站点" }).click();
  await expect.poll(() => held.length).toBeGreaterThan(previousReads);
  for (const route of held.slice(previousReads))
    await route.fulfill({ json: { revision: 1, preferred_id: "", items: [] } });
  await expect(page.getByRole("combobox", { name: "配置模板" })).toBeEnabled();
  await expect(input).toHaveValue("rt_isolated");
});

test("关闭来源读取弹窗后重新打开会重新请求且读取就绪前不能保存", async ({ page }) => {
  await setup(page);
  const held: Route[] = [];
  await page.route("**/api/account-workbench/template-source/41", (route) => {
    held.push(route);
  });
  await page.goto("/account-workbench?tab=templates");
  await page.getByRole("button", { name: "重新同步", exact: true }).click();
  await expect(page.getByRole("status", { name: "正在读取来源配置" })).toBeVisible();
  await expect(page.getByRole("button", { name: "保存模板" })).toBeDisabled();
  await expect.poll(() => held.length).toBeGreaterThan(0);
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const previousReads = held.length;
  await page.getByRole("button", { name: "重新同步", exact: true }).click();
  await expect.poll(() => held.length).toBeGreaterThan(previousReads);
  await expect(page.getByRole("button", { name: "保存模板" })).toBeDisabled();
  for (const route of held.slice(previousReads)) await route.fulfill({ json: template });
  await expect(page.getByRole("button", { name: "保存模板" })).toBeEnabled();
  await page.getByRole("button", { name: "取消", exact: true }).click();
});
