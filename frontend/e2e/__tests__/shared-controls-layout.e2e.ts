import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test, type Page } from "@playwright/test";

let bundleDirectory: string;
test.beforeAll(() => {
  bundleDirectory = mkdtempSync(join(tmpdir(), "console-shared-controls-"));
  execFileSync(
    "bun",
    ["build", "e2e/__tests__/fixtures/shared-controls-layout.tsx", "--outdir", bundleDirectory],
    { cwd: process.cwd(), stdio: "pipe" },
  );
});
test.afterAll(() => rmSync(bundleDirectory, { recursive: true, force: true }));

async function openFixture(page: Page, surface: string, theme: string | null): Promise<void> {
  await page.route("**/api/**", (route) =>
    route.fulfill({ json: { initialized: false, configuration_errors: [] } }),
  );
  await page.goto("/");
  await expect(page.getByRole("button", { name: "完成初始化" })).toBeVisible();
  const css = await page.evaluate(() =>
    Array.from(document.styleSheets)
      .flatMap((sheet) => Array.from(sheet.cssRules).map((rule) => rule.cssText))
      .join("\n"),
  );
  await page.route("**/shared-controls.html?*", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: '<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><main id="controls-root" style="padding:16px;max-width:480px;margin:0 auto"></main></body></html>',
    }),
  );
  await page.goto(`/shared-controls.html?surface=${surface}`);
  await page.addStyleTag({ content: css });
  await page.evaluate(
    (value) => document.documentElement.classList.toggle("dark", value === "dark"),
    theme,
  );
  await page.addScriptTag({
    path: join(bundleDirectory, "shared-controls-layout.js"),
    type: "module",
  });
}

test("窄屏菜单的无空格长名称完整换行，不超出视口", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await openFixture(page, "menu-long", colorScheme);
  await page.getByRole("button", { name: "更多操作" }).click();
  const menu = page.getByRole("menu");
  await expect(menu).toBeInViewport({ ratio: 1 });
  expect(await menu.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.getByRole("menuitem").click();
  await expect(page.getByRole("status")).toHaveText("已选择账号");
});

test("矮屏长菜单在内部滚动，键盘可选择最后一项并返回触发按钮", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 390, height: 240 });
  await openFixture(page, "menu-many", colorScheme);
  const trigger = page.getByRole("button", { name: "更多操作" });
  await trigger.focus();
  await trigger.press("ArrowDown");
  const menu = page.getByRole("menu");
  await expect(menu).toBeInViewport({ ratio: 1 });
  await page.keyboard.press("End");
  const last = page.getByRole("menuitem", { name: "操作 16", exact: true });
  await expect(last).toBeFocused();
  await expect(last).toBeInViewport({ ratio: 1 });
  await page.keyboard.press("Enter");
  await expect(page.getByRole("status")).toHaveText("已执行操作 16");
  await expect(trigger).toBeFocused();
});

test("中等宽度卡片的多个操作换行，标题保持可读宽度", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 460, height: 568 });
  await openFixture(page, "card", colorScheme);
  const card = page.locator('[data-slot="card"]');
  expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const title = page.getByText("自动巡检运行记录", { exact: true });
  const bounds = (await title.boundingBox())!;
  expect(bounds.height).toBeLessThanOrEqual(40);
  for (const button of await card.getByRole("button").all()) {
    await expect(button).toBeInViewport({ ratio: 1 });
  }
});

test("窄屏选择超长账号后移除入口保留尺寸且能够清空选择", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 320, height: 568 });
  await openFixture(page, "selection", colorScheme);
  const trigger = page.getByRole("combobox", { name: "账号筛选" });
  expect(await trigger.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  const remove = trigger.getByRole("button", { name: "移除" });
  await expect(remove).toHaveCSS("width", "20px");
  await remove.click();
  await expect(trigger).toHaveText("账号筛选");
});
