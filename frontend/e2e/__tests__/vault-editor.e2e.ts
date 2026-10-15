import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/auth-recovery/config": {
        auth_records: [],
        vault_entries: [
          {
            id: "vault-independent-id",
            entry: "主站管理员",
            hosts: ["api.example.test"],
            has_username: true,
            has_password: true,
            username_is_email: true,
            header_names: ["X-Private-" + "Long".repeat(30)],
          },
        ],
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

test("编辑凭据时名称可改、字段按屏宽排列，长请求头不撑宽弹窗", async ({
  page,
  viewport,
}, testInfo) => {
  await page.goto("/vault");
  await page.getByRole("button", { name: "编辑凭据", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑凭据" });
  await page.getByRole("textbox", { name: "凭据名称" }).fill("重命名后的凭据");
  await expect(page.getByRole("textbox", { name: "凭据名称" })).toHaveValue("重命名后的凭据");
  await expect(page.getByRole("textbox", { name: "Headers JSON" })).toBeVisible();
  const username = await page.getByLabel("用户名", { exact: true }).boundingBox();
  const password = await page.getByLabel("密码", { exact: true }).boundingBox();
  if (viewport!.width < 640) expect(password!.y).toBeGreaterThan(username!.y);
  else expect(Math.abs(password!.y - username!.y)).toBeLessThan(2);
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "保存修改" })).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: testInfo.outputPath("vault-editor.png") });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("低高度编辑窗口中内容独立滚动，保存和取消始终可达", async ({ page, viewport }) => {
  await page.setViewportSize({ width: viewport!.width, height: 480 });
  await page.goto("/vault");
  await page.getByRole("button", { name: "编辑凭据", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑凭据" });
  await expect(dialog.getByRole("button", { name: "保存修改" })).toBeInViewport({ ratio: 1 });
  const body = dialog.locator('[data-slot="dialog-body"]');
  expect(await body.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
  await dialog.getByRole("button", { name: "清除 Headers" }).scrollIntoViewIfNeeded();
  await expect(dialog.getByRole("button", { name: "清除 Headers" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "保存修改" })).toBeInViewport({ ratio: 1 });
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});

test("凭据重命名保存时仍提交原 ID", async ({ page }) => {
  let saved: unknown;
  await page.route("**/api/auth-recovery/vault-entry", async (route) => {
    saved = route.request().postDataJSON();
    await route.fulfill({ json: { ok: true } });
  });
  await page.goto("/vault");
  await page.getByRole("button", { name: "编辑凭据", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑凭据" });
  await dialog.getByRole("textbox", { name: "凭据名称" }).fill("新的显示名称");
  await dialog.getByRole("button", { name: "保存修改" }).click();
  await expect(dialog).toHaveCount(0);
  expect(saved).toMatchObject({ id: "vault-independent-id", entry: "新的显示名称" });
});
