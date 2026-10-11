import { expect, test } from "@playwright/test";

import { importPreviewFixture } from "../../src/features/account-workbench/__tests__/import-preview-fixture";
import { pageFixtures } from "./fixtures/page-shell";

for (const compact of [false, true]) {
  test(`${compact ? "320px 低高度" : "默认窗口"}导入分页保留整批确认、长账号和等待状态`, async ({
    page,
  }, info) => {
    if (compact) await page.setViewportSize({ width: 320, height: 568 });
    await page.addInitScript(
      (theme) => {
        localStorage.setItem("sub2api-console-theme", theme);
      },
      info.project.name === "desktop-light" ? "light" : "dark",
    );
    const errors: Error[] = [];
    page.on("pageerror", (error) => errors.push(error));
    const preview = importPreviewFixture(500);
    const longEmail = `${"long-account-".repeat(30)}@example.test`;
    preview.items[0].email = longEmail;
    let submitted: unknown;
    let releaseStart: (() => void) | undefined;
    const startGate = new Promise<void>((resolve) => {
      releaseStart = resolve;
    });
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "导入分页测试" },
        "/api/dictionaries": { items: [] },
        "/api/account-workbench/templates": { revision: 0, preferred_id: "", items: [] },
        "/api/account-workbench/accounts": [],
        "/api/account-workbench/runs": [],
        "/api/account-workbench/preview": preview,
      };
      if (path === "/api/account-workbench/runs" && route.request().method() === "POST") {
        submitted = route.request().postDataJSON();
        await startGate;
        await route.fulfill({ json: { id: "run-batch" } });
      } else if (path.endsWith("/events")) {
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      } else if (path in fixtures) {
        await route.fulfill({ json: fixtures[path] });
      } else {
        await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      }
    });
    try {
      await page.goto("/account-workbench");
      await page.getByRole("textbox", { name: "账号资料" }).fill("rt_pagination-fixture");
      await page.getByRole("button", { name: "解析并预览" }).click();
      const dialog = page.getByRole("dialog", { name: "确认本批导入" });
      const table = dialog.getByRole("table", { name: "本批账号预览" });
      await expect(table.getByRole("row")).toHaveCount(21);
      await expect(table.getByRole("cell", { name: longEmail, exact: true })).toBeVisible();
      expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
        true,
      );
      const container = dialog.locator('[data-slot="table-container"]');
      expect(await container.evaluate((element) => getComputedStyle(element).overflowX)).toBe(
        "auto",
      );
      if (compact || info.project.name === "mobile-dark") {
        expect(
          await container.evaluate((element) => element.scrollWidth > element.clientWidth),
        ).toBe(true);
      }
      const body = dialog.locator('[data-slot="dialog-body"]');
      await body.evaluate((element) => element.scrollTo(0, element.scrollHeight));
      await dialog.getByRole("button", { name: "转到下一页" }).focus();
      await page.keyboard.press("Enter");
      await expect(
        table.getByRole("cell", { name: "account-21@example.test", exact: true }),
      ).toBeVisible();
      await expect(table.getByRole("cell", { name: "21", exact: true })).toBeVisible();
      await expect(dialog.getByRole("heading", { name: "确认本批导入" })).toBeInViewport();
      const confirm = dialog.getByRole("button", { name: "确认并开始" });
      await expect(confirm).toBeInViewport({ ratio: 1 });
      await page.screenshot({
        path: info.outputPath(`preview-${compact ? "compact" : "default"}.png`),
      });
      await confirm.click();
      await expect.poll(() => submitted).toEqual({ id: preview.id, revision: preview.revision });
      await expect(dialog.getByRole("status", { name: "正在创建账号处理任务" })).toBeVisible();
      await expect(confirm).toBeDisabled();
      await expect(dialog.getByRole("button", { name: "返回修改" })).toBeDisabled();
      await expect(dialog.getByRole("progressbar")).toHaveCount(0);
      await dialog.getByRole("button", { name: "转到上一页" }).click();
      await expect(table.getByRole("cell", { name: longEmail, exact: true })).toBeVisible();
      await page.screenshot({
        path: info.outputPath(`pending-${compact ? "compact" : "default"}.png`),
      });
      releaseStart?.();
      await expect(dialog).toHaveCount(0);
      await expect(page.getByRole("tab", { name: "处理记录", exact: true })).toHaveAttribute(
        "aria-selected",
        "true",
      );
      expect(errors).toEqual([]);
    } finally {
      releaseStart?.();
    }
  });
}

test("其他页账号错误在首页可见并阻止整批确认", async ({ page }) => {
  const preview = importPreviewFixture(41);
  preview.errors = [{ index: 40, message: "账号资料缺少官方工作区" }];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "导入错误测试" },
      "/api/dictionaries": { items: [] },
      "/api/account-workbench/templates": { revision: 0, preferred_id: "", items: [] },
      "/api/account-workbench/preview": preview,
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/account-workbench");
  await page.getByRole("textbox", { name: "账号资料" }).fill("rt_error-fixture");
  await page.getByRole("button", { name: "解析并预览" }).click();
  const dialog = page.getByRole("dialog", { name: "确认本批导入" });
  await expect(dialog.getByText("第 41 项：账号资料缺少官方工作区")).toBeVisible();
  await expect(
    dialog.getByRole("cell", { name: "account-41@example.test", exact: true }),
  ).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "确认并开始" })).toBeDisabled();
  await dialog.getByRole("button", { name: "返回修改" }).click();
  await expect(page.getByRole("textbox", { name: "账号资料" })).toHaveValue("rt_error-fixture");
});
