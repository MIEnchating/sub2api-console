import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "@playwright/test";

let bundleDirectory: string;
test.beforeAll(() => {
  bundleDirectory = mkdtempSync(join(tmpdir(), "console-browser-fixture-"));
  execFileSync(
    "bun",
    [
      "build",
      "e2e/__tests__/fixtures/browser-login.tsx",
      "--outdir",
      bundleDirectory,
      "--define",
      "import.meta.env={}",
    ],
    { cwd: process.cwd(), stdio: "pipe" },
  );
});
test.afterAll(() => rmSync(bundleDirectory, { recursive: true, force: true }));

test("验证码失败可重新加载，画面与按钮在桌面和手机中可操作且不溢出", async ({
  page,
  colorScheme,
}) => {
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
  await page.route("**/browser-login-fixture", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: '<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><main id="browser-root"></main></body></html>',
    }),
  );
  await page.goto("/browser-login-fixture");
  await page.addStyleTag({ content: css });
  await page.evaluate(
    (theme) => document.documentElement.classList.toggle("dark", theme === "dark"),
    colorScheme,
  );
  const image = await page.evaluate(() => {
    const canvas = document.createElement("canvas");
    canvas.width = 1100;
    canvas.height = 760;
    const context = canvas.getContext("2d")!;
    context.fillStyle = "#f4f5f7";
    context.fillRect(0, 0, 1100, 760);
    context.fillStyle = "#111827";
    context.font = "28px sans-serif";
    context.fillText("Login", 390, 180);
    context.font = "18px sans-serif";
    context.fillText("Email", 390, 240);
    context.fillText("Password", 390, 330);
    context.strokeStyle = "#71717a";
    context.strokeRect(390, 255, 320, 42);
    context.strokeRect(390, 345, 320, 42);
    context.fillStyle = "#b91c1c";
    context.fillText("Verification failed", 390, 445);
    return canvas.toDataURL("image/jpeg");
  });
  let reloaded = false;
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/auth-recovery/browser**", async (route) => {
    if (route.request().url().endsWith("/input")) {
      expect(route.request().postDataJSON()).toEqual({ kind: "reload" });
      reloaded = true;
      await pending;
      await route.fulfill({ json: { accepted: true } });
      return;
    }
    if (route.request().method() === "DELETE") {
      await route.fulfill({ json: { cancelled: true } });
      return;
    }
    await route.fulfill({
      json: {
        id: "isolated-browser",
        task_id: "isolated-task",
        host: "login.example.test",
        status: "waiting",
        message: "等待验证",
        expires_at: "2030-01-01T00:00:00Z",
        image,
        width: 1100,
        height: 760,
        challenge_code: reloaded ? undefined : "600010",
      },
    });
  });
  await page.addScriptTag({ path: join(bundleDirectory, "browser-login.js"), type: "module" });
  await page.getByRole("button", { name: "打开浏览器手动验证" }).click();
  const dialog = page.getByRole("dialog", { name: "浏览器手动验证 · login.example.test" });
  const surface = dialog.getByRole("button", { name: "上游登录页面" });
  await expect(surface).toBeVisible();
  await expect(page.getByText(/Cloudflare 验证失败（600010）/)).toHaveCount(1);
  await expect(page.getByText(/Cloudflare 验证失败（600010）/)).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "刷新验证页面" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "关闭验证" })).toBeInViewport({ ratio: 1 });
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("browser-challenge.png") });
  const reload = dialog.getByRole("button", { name: "刷新验证页面" });
  await reload.click();
  await expect(surface).toHaveAttribute("aria-disabled", "true");
  await expect(dialog.getByRole("button", { name: "登录完成，复核并保存" })).toBeDisabled();
  await expect(dialog.getByRole("button", { name: "关闭验证" })).toBeEnabled();
  release();
  await expect(reload).toBeEnabled();
  await expect(surface).toHaveAttribute("aria-disabled", "false");
  await dialog.getByRole("button", { name: "关闭验证" }).click();
  await expect(dialog).not.toBeVisible();
});
