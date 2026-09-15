import { expect, test } from "@playwright/test";
import type {
  WorkbenchOAuthBatch,
  WorkbenchOAuthBatchPreview,
  WorkbenchOAuthBatchInput,
} from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("批量授权在桌面和手机确认范围后逐项登录并导入成功账号", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const fixture = await installWorkbenchFixture(page);
  let submitted: WorkbenchOAuthBatchInput | null = null;
  let confirmed: unknown;
  let cancelled = false;
  let finished = false;
  const preview: WorkbenchOAuthBatchPreview = {
    id: "batch-preview",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    errors: [],
    items: [
      {
        index: 0,
        email: "operator@example.test",
        workspace_id: "workspace-" + "long".repeat(20),
        has_password: true,
        has_totp: false,
        sms_provider: "custom",
        status: "queued",
        message: "等待授权",
      },
      {
        index: 1,
        email: "second@example.test",
        has_password: false,
        has_totp: true,
        sms_provider: "custom",
        status: "queued",
        message: "等待授权",
      },
    ],
  };
  await page.route("**/api/account-workbench/oauth/oauth-fixture/finish", async (route) => {
    finished = true;
    await route.fallback();
  });
  await page.route("**/api/account-workbench/oauth-batches**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const method = route.request().method();
    if (path === "/api/account-workbench/oauth-batches/preview") {
      submitted = route.request().postDataJSON() as WorkbenchOAuthBatchInput;
      return route.fulfill({ json: preview });
    }
    if (method === "DELETE") {
      cancelled = true;
      return route.fulfill({ json: { cancelled: true, deleted: true } });
    }
    if (path === "/api/account-workbench/oauth-batches")
      confirmed = route.request().postDataJSON() as unknown;
    if (path === "/api/account-workbench/oauth-batches/batch-fixture/preview")
      return route.fulfill({
        json: {
          id: "oauth-preview",
          target: preview.target,
          expires_at: preview.expires_at,
          errors: [],
          check_after_import: false,
          model: "",
          items: [
            {
              id: "0",
              index: 0,
              name: "operator",
              email: "operator@example.test",
              plan_type: "plus",
              template_id: "",
              template_name: "",
              template_revision: 0,
              group_ids: [],
              duplicate: false,
            },
          ],
        },
      });
    const value: WorkbenchOAuthBatch = {
      id: "batch-fixture",
      task_id: "batch-task",
      status: finished ? "authorized" : "running",
      expires_at: preview.expires_at,
      message: finished ? "1 个账号授权成功，1 个账号授权失败" : "正在逐项授权",
      available: finished ? 1 : 0,
      current_oauth_id: finished ? undefined : "oauth-fixture",
      items: preview.items.map((item, index) => {
        if (!finished) return { ...item, status: "running", message: "正在授权" };
        if (index === 0) return { ...item, status: "succeeded", message: "授权成功，等待导入确认" };
        return { ...item, status: "failed", message: "账号验证未完成，请重新授权" };
      }),
    };
    return route.fulfill({ json: value });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  const batchTab = page.getByRole("tab", { name: "批量授权", exact: true });
  await batchTab.focus();
  await page.keyboard.press("Enter");
  await expect(batchTab).toHaveAttribute("aria-selected", "true");
  await page.getByRole("button", { name: "填写批量账号" }).click();
  const dialog = page.getByRole("dialog", { name: "批量授权账号" });
  await dialog
    .getByRole("textbox", { name: "批量授权内容" })
    .fill("operator@example.test----private-password\nsecond@example.test");
  await dialog.getByRole("combobox", { name: "短信验证码" }).click();
  await page.getByRole("option", { name: "自定义接码", exact: true }).click();
  await dialog
    .getByRole("textbox", { name: "自定义接码列表" })
    .fill("+12025550123----https://sms.example.test/code");
  await dialog.getByRole("checkbox", { name: "我确认绑定接码手机号，并承担供应商费用" }).check();
  await expect(dialog.getByRole("heading", { name: "批量授权账号" })).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "解析授权账号" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("batch-authorize-form.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "解析授权账号" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("region", { name: "批量授权预览" })).toBeVisible();
  expect(submitted).toMatchObject({
    content: "operator@example.test----private-password\nsecond@example.test",
    sms: { provider: "custom", confirmed: true },
  });
  expect(confirmed).toBeUndefined();
  await page.getByRole("button", { name: "确认授权 2 个账号" }).click();
  const confirmation = page.getByRole("dialog", { name: "确认批量授权" });
  await expect(confirmation).toContainText("承担供应商费用");
  await confirmation.getByRole("button", { name: "开始批量授权" }).click();
  await expect.poll(() => confirmed).toEqual({ preview_id: "batch-preview", confirmed: true });
  const browser = page.getByRole("button", { name: "上游登录页面" });
  await expect(browser).toBeVisible();
  await browser.focus();
  await page.keyboard.type("operator@example.test");
  await expect.poll(() => fixture.inputs.length).toBeGreaterThan(0);
  await page.getByRole("button", { name: "登录完成，验证授权" }).click();
  await expect(page.getByRole("button", { name: "预览授权账号" })).toBeVisible();
  await expect(page.getByRole("region", { name: "批量授权进度" })).toContainText("可导入账号：1");
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("batch-authorize-results.png"),
    animations: "disabled",
  });
  await page.getByRole("button", { name: "预览授权账号" }).click();
  await page.getByRole("button", { name: "确认导入 1 个账号" }).click();
  await page.getByRole("button", { name: "创建导入任务" }).click();
  await expect
    .poll(() => fixture.imports)
    .toEqual([{ preview_id: "oauth-preview", confirmed: true }]);
  await expect.poll(() => cancelled).toBe(true);
  await expect(page.getByRole("status", { name: "等待导入授权账号" })).toBeVisible();
});
