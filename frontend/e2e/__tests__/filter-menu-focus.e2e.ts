import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "@playwright/test";

let bundleDirectory: string;

test.beforeAll(() => {
  bundleDirectory = mkdtempSync(join(tmpdir(), "console-filter-focus-"));
  execFileSync(
    "bun",
    [
      "build",
      "e2e/__tests__/fixtures/filter-menu.tsx",
      "--define",
      "import.meta.env={}",
      "--outdir",
      bundleDirectory,
    ],
    {
      cwd: process.cwd(),
      stdio: "pipe",
    },
  );
});
test.afterAll(() => rmSync(bundleDirectory, { recursive: true, force: true }));

test.beforeEach(async ({ page, colorScheme }) => {
  // Load application styles only; every API request is isolated from the backend.
  await page.route("**/api/**", (route) => route.abort());
  await page.goto("/");
  await expect(page.locator("body")).toHaveCSS("font-family", /Public Sans Variable/);
  const css = await page.evaluate(() =>
    Array.from(document.styleSheets)
      .flatMap((sheet) => Array.from(sheet.cssRules).map((rule) => rule.cssText))
      .join("\n"),
  );
  await page.route("**/filter-focus.html", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: '<html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="filter-root" style="padding:32px"></main></body></html>',
    }),
  );
  await page.goto("/filter-focus.html");
  page.on("pageerror", (error) => {
    throw error;
  });
  await page.addStyleTag({ content: css });
  await page.evaluate(
    (theme) => document.documentElement.classList.toggle("dark", theme === "dark"),
    colorScheme,
  );
  await page.addScriptTag({ path: join(bundleDirectory, "filter-menu.js"), type: "module" });
});

test("点击选项及清除后可继续键盘筛选，关闭后回到触发按钮", async ({ page }) => {
  const trigger = page.getByRole("button", { name: "分组筛选" });
  await trigger.click();
  const search = page.getByRole("combobox", { name: "搜索分组" });
  await page.getByRole("option", { name: "Codex" }).click();
  await expect(search).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("option", { name: "Claude" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.getByRole("button", { name: "清除筛选" }).click();
  await expect(search).toBeFocused();
  await page.keyboard.type("Codex");
  await expect(page.getByRole("option")).toHaveCount(1);
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("搜索文本支持 Home 和 End 编辑，空搜索仍可定位首尾选项", async ({ page }) => {
  await page.getByRole("button", { name: "分组筛选" }).click();
  const search = page.getByRole("combobox", { name: "搜索分组" });
  await search.press("End");
  await expect(search).toHaveAttribute(
    "aria-activedescendant",
    (await page.getByRole("option", { name: "Claude" }).getAttribute("id")) as string,
  );
  await search.press("Home");
  await expect(search).toHaveAttribute(
    "aria-activedescendant",
    (await page.getByRole("option", { name: "默认" }).getAttribute("id")) as string,
  );
  await page.keyboard.type("od");
  await page.keyboard.press("Home");
  await page.keyboard.type("C");
  await page.keyboard.press("End");
  await page.keyboard.type("ex");
  await expect(search).toHaveValue("Codex");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("option", { name: "Codex" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
});
