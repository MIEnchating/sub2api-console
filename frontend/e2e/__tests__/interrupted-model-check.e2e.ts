import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "@playwright/test";

let bundleDirectory: string;
test.beforeAll(() => {
  bundleDirectory = mkdtempSync(join(tmpdir(), "console-interrupted-check-"));
  execFileSync(
    "bun",
    [
      "build",
      "e2e/__tests__/fixtures/interrupted-model-check.tsx",
      "--define",
      "import.meta.env={}",
      "--outdir",
      bundleDirectory,
    ],
    { cwd: process.cwd(), stdio: "pipe" },
  );
});
test.afterAll(() => rmSync(bundleDirectory, { recursive: true, force: true }));

for (const status of ["cancelled", "failed"] as const) {
  test(`${status} 检测在桌面和手机保留明细、终态及分页`, async ({
    page,
    colorScheme,
  }, testInfo) => {
    const mobile = testInfo.project.name === "mobile-dark";
    if (mobile) await page.setViewportSize({ width: 320, height: 568 });
    const errors: string[] = [];
    page.on("pageerror", (error) => {
      errors.push(error.message);
      throw error;
    });
    await page.route("**/api/**", (route) => route.fulfill({ status: 503, json: {} }));
    await page.goto("/");
    const css = await page.evaluate(() =>
      Array.from(document.styleSheets)
        .flatMap((sheet) => Array.from(sheet.cssRules).map((rule) => rule.cssText))
        .join("\n"),
    );
    await page.route("**/interrupted-check-fixture?*", (route) =>
      route.fulfill({
        contentType: "text/html",
        body: '<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><main id="result-root" style="height:100dvh;padding:16px"></main></body></html>',
      }),
    );
    await page.goto(`/interrupted-check-fixture?status=${status}`);
    await page.addStyleTag({ content: css });
    await page.evaluate(
      (theme) => document.documentElement.classList.toggle("dark", theme === "dark"),
      colorScheme,
    );
    await page.addScriptTag({
      path: join(bundleDirectory, "interrupted-model-check.js"),
      type: "module",
    });

    const heading = page.getByText(status === "cancelled" ? "检测已取消" : "检测失败", {
      exact: true,
    });
    await expect(heading).toBeVisible();
    await expect(page.getByText("检测完成", { exact: true })).toHaveCount(0);
    if (status === "failed") {
      await expect(page.getByText("检测连接中断，请重新提交")).toBeVisible();
    }
    const results = page.getByTestId(
      mobile ? "model-check-result-mobile-list" : "model-check-result-desktop-table",
    );
    await expect(results).toBeVisible();
    await expect(results.getByText("已完成账号 1", { exact: true })).toBeVisible();
    await expect(results.getByText("已完成账号 21", { exact: true })).toHaveCount(0);
    await page.screenshot({
      path: testInfo.outputPath("interrupted-results-first.png"),
      fullPage: true,
    });

    const next = page.getByRole("button", { name: "转到下一页", exact: true });
    await next.focus();
    await page.keyboard.press("Enter");
    await expect(results.getByText("已完成账号 21", { exact: true })).toBeVisible();
    await expect(results.getByText("3/3", { exact: true })).toBeVisible();
    await expect(heading).toBeVisible();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath("interrupted-results-last.png"),
      fullPage: true,
    });
    expect(errors).toEqual([]);
  });
}
