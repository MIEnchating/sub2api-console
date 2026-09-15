import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "@playwright/test";

export function useJsonEditorFixture(): void {
  let bundleDirectory: string;
  test.beforeAll(() => {
    bundleDirectory = mkdtempSync(join(tmpdir(), "console-json-editor-"));
    execFileSync(
      "bun",
      ["build", "e2e/__tests__/fixtures/json-editor.tsx", "--outdir", bundleDirectory],
      { cwd: process.cwd(), stdio: "pipe" },
    );
  });
  test.afterAll(() => rmSync(bundleDirectory, { recursive: true, force: true }));

  test.beforeEach(async ({ page, colorScheme }) => {
    page.on("pageerror", (error) => {
      throw error;
    });
    await page.route("**/api/**", (route) => route.fulfill({ status: 503, json: {} }));
    await page.goto("/");
    const css = await page.evaluate(() =>
      Array.from(document.styleSheets)
        .flatMap((sheet) => Array.from(sheet.cssRules).map((rule) => rule.cssText))
        .join("\n"),
    );
    await page.route("**/json-editor-fixture", (route) =>
      route.fulfill({
        contentType: "text/html",
        body: '<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><main id="editor-root" style="padding:16px;max-width:640px;margin:auto"></main></body></html>',
      }),
    );
    await page.goto("/json-editor-fixture");
    await page.addStyleTag({ content: css });
    await page.evaluate(
      (theme) => document.documentElement.classList.toggle("dark", theme === "dark"),
      colorScheme,
    );
    await page.addScriptTag({ path: join(bundleDirectory, "json-editor.js"), type: "module" });
  });
}
