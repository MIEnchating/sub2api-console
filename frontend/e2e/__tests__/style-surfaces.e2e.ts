import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test, type Page } from "@playwright/test";

let bundleDirectory: string;
test.beforeAll(() => {
  bundleDirectory = mkdtempSync(join(tmpdir(), "console-style-surfaces-"));
  execFileSync(
    "bun",
    ["build", "e2e/__tests__/fixtures/style-surfaces.tsx", "--outdir", bundleDirectory],
    { cwd: process.cwd(), stdio: "pipe" },
  );
});
test.afterAll(() => rmSync(bundleDirectory, { recursive: true, force: true }));

async function openSurface(page: Page, surface: string, theme: string | null): Promise<void> {
  page.on("pageerror", (error) => {
    throw error;
  });
  await page.route("**/api/**", () => {});
  await page.goto("/");
  await expect(page.getByRole("status")).toBeVisible();
  const css = await page.evaluate(() =>
    Array.from(document.styleSheets)
      .flatMap((sheet) => Array.from(sheet.cssRules).map((rule) => rule.cssText))
      .join("\n"),
  );
  await page.route("**/style-audit.html?*", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: '<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><main id="audit-root" style="padding:16px;max-width:480px;margin:0 auto"></main></body></html>',
    }),
  );
  await page.goto(`/style-audit.html?surface=${surface}`);
  await page.addStyleTag({ content: css });
  await page.evaluate(
    (value) => document.documentElement.classList.toggle("dark", value === "dark"),
    theme,
  );
  await page.addScriptTag({ path: join(bundleDirectory, "style-surfaces.js"), type: "module" });
}

test("长说明浮层保持在窄屏内，正文完整换行而不水平溢出", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 320, height: 480 });
  await openSurface(page, "tooltip", colorScheme);
  await page.getByRole("button", { name: "查看模型说明" }).hover();
  const tooltip = page.locator('[data-slot="tooltip-content"]');
  await expect(tooltip).toBeVisible();
  expect(await tooltip.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await expect(tooltip).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: test.info().outputPath("tooltip.png") });
});

test("弹窗多个长操作按钮可以换行，按钮文字不溢出且保持可点击", async ({ page, colorScheme }) => {
  await openSurface(page, "dialog", colorScheme);
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  for (const button of await dialog.locator('[data-slot="dialog-footer"] button').all()) {
    await button.click({ trial: true });
    expect(await button.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
  }
  await page.screenshot({ path: test.info().outputPath("dialog.png") });
});

test("徽标在窄容器内不撑宽布局，文字行高不被固定高度裁切", async ({ page, colorScheme }) => {
  await openSurface(page, "badge", colorScheme);
  const badges = page.locator('[data-slot="badge"]');
  await expect(badges).toHaveCount(2);
  await expect(badges.first()).toHaveCSS("height", "20px");
  await expect(badges.last()).toHaveCSS("height", "20px");
  await expect(badges.last()).toHaveAttribute("title", "long-model-name-".repeat(25));
  expect(
    await badges.first().evaluate((element) => element.scrollHeight <= element.clientHeight),
  ).toBe(true);
  expect(
    await badges
      .last()
      .evaluate(
        (element) =>
          element.getBoundingClientRect().width <=
          element.parentElement!.getBoundingClientRect().width,
      ),
  ).toBe(true);
  await page.screenshot({ path: test.info().outputPath("badges.png") });
});

test("低高度弹窗滚动长正文时标题、关闭和底部操作保持可见", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 390, height: 480 });
  await openSurface(page, "scroll-dialog", colorScheme);
  const dialog = page.getByRole("dialog", { name: "确认影响范围" });
  const title = dialog.getByRole("heading", { name: "确认影响范围" });
  const confirm = dialog.getByRole("button", { name: "确认更新", exact: true });
  const close = dialog.getByRole("button", { name: "关闭", exact: true });
  await close.click({ trial: true });
  await expect(confirm).toBeInViewport({ ratio: 1 });
  const titleBounds = await title.boundingBox();
  const footerBounds = await confirm.boundingBox();
  const body = dialog.locator('[data-slot="dialog-body"]');
  await body.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect.poll(() => body.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  expect(await dialog.evaluate((element) => element.scrollHeight <= element.clientHeight)).toBe(
    true,
  );
  expect(await title.boundingBox()).toEqual(titleBounds);
  expect(await confirm.boundingBox()).toEqual(footerBounds);
  await expect(title).toBeInViewport({ ratio: 1 });
  await confirm.click({ trial: true });
  await close.click({ trial: true });
  await page.screenshot({ path: test.info().outputPath("dialog-footer-scrolled.png") });
});

test("短正文弹窗按内容收缩且保留底部操作", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 390, height: 480 });
  await openSurface(page, "short-dialog", colorScheme);
  const dialog = page.getByRole("dialog", { name: "确认影响范围" });
  await expect(dialog).toBeVisible();
  expect((await dialog.boundingBox())!.height).toBeLessThan(300);
  expect(await dialog.evaluate((element) => element.scrollHeight <= element.clientHeight)).toBe(
    true,
  );
  await dialog.getByRole("button", { name: "确认更新", exact: true }).click({ trial: true });
});

test("选择超长模型后移除按钮保持完整可见，键盘可以移除已选项", async ({ page, colorScheme }) => {
  await openSurface(page, "select", colorScheme);
  const trigger = page.getByRole("combobox", { name: "选择模型" });
  const remove = page.getByRole("button", { name: "移除", exact: true });
  await expect(remove).toBeVisible();
  expect(await trigger.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await remove.focus();
  await page.keyboard.press("Enter");
  await expect(remove).toHaveCount(0);
});

test("局部读取变为失败重试时状态区高度保持一致", async ({ page, colorScheme }) => {
  await openSurface(page, "retry", colorScheme);
  const region = page.getByRole("region", { name: "内容状态" });
  await expect(region.getByRole("status")).toBeVisible();
  const initial = (await region.boundingBox())!.height;
  await page.getByRole("button", { name: "切换状态" }).click();
  await expect(page.getByRole("button", { name: "重新读取" })).toBeVisible();
  expect((await region.boundingBox())!.height).toBe(initial);
});
