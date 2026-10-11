import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "@playwright/test";

let bundleDirectory: string;
const longName = "键盘可查看的完整账号名称";

test.beforeAll(() => {
  bundleDirectory = mkdtempSync(join(tmpdir(), "console-table-overflow-"));
  execFileSync(
    "bun",
    ["build", "e2e/__tests__/fixtures/table-overflow.tsx", "--outdir", bundleDirectory],
    {
      cwd: process.cwd(),
      stdio: "pipe",
    },
  );
});
test.afterAll(() => rmSync(bundleDirectory, { recursive: true, force: true }));

test.beforeEach(async ({ page, colorScheme }) => {
  await page.route("**/api/**", (route) => route.abort());
  await page.goto("/");
  await expect(page.locator("body")).toHaveCSS("font-family", /Public Sans Variable/);
  const css = await page.evaluate(() =>
    Array.from(document.styleSheets)
      .flatMap((sheet) => Array.from(sheet.cssRules).map((rule) => rule.cssText))
      .join("\n"),
  );
  await page.route("**/table-overflow.html", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: '<html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="table-root" style="padding:80px 16px"></main></body></html>',
    }),
  );
  await page.goto("/table-overflow.html");
  page.on("pageerror", (error) => {
    throw error;
  });
  await page.addStyleTag({ content: css });
  await page.evaluate(
    (theme) => document.documentElement.classList.toggle("dark", theme === "dark"),
    colorScheme,
  );
  await page.addScriptTag({ path: join(bundleDirectory, "table-overflow.js"), type: "module" });
});

test("键盘跳过未截断文字，完整查看长名称后仍可操作账号", async ({ page }) => {
  const trigger = page.getByRole("table", { name: "账号" }).getByText(longName, { exact: true });
  await expect(trigger).toHaveAttribute("tabindex", "0");
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "编辑短账号" })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(trigger).toBeFocused();
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toHaveText(longName);
  await expect(trigger).toHaveAccessibleDescription(longName);
  await expect(tooltip).toBeInViewport({ ratio: 1 });
  await expect(trigger).not.toHaveCSS("box-shadow", "none");
  await page.screenshot({ path: test.info().outputPath("keyboard-tooltip.png") });
  await page.keyboard.press("Escape");
  await expect(tooltip).toBeHidden();
  await expect(trigger).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "编辑长账号" })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByText("已编辑长账号")).toBeVisible();
});

test("容器加宽后说明关闭，收窄后恢复入口，名称缩短后移除入口", async ({ page }) => {
  const trigger = page.getByRole("table", { name: "账号" }).getByText(longName, { exact: true });
  await expect(trigger).toHaveAttribute("tabindex", "0");
  await page.keyboard.press("Tab");
  await page.keyboard.press("Tab");
  await expect(page.getByRole("tooltip")).toBeVisible();
  await page.evaluate(() => document.documentElement.style.setProperty("--fixture-width", "320px"));
  await expect(trigger).toHaveAttribute("tabindex", "-1");
  await expect(page.getByRole("tooltip")).toBeHidden();
  await expect(trigger).toBeFocused();
  await page.evaluate(() => document.documentElement.style.setProperty("--fixture-width", "160px"));
  await expect(trigger).toHaveAttribute("tabindex", "0");
  await page.getByRole("button", { name: "缩短名称" }).click();
  await expect(page.getByText("短名称二", { exact: true })).toHaveAttribute("tabindex", "-1");
});
