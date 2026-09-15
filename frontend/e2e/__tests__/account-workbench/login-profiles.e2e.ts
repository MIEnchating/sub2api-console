import { expect, test } from "@playwright/test";
import type {
  WorkbenchLoginProfile,
  WorkbenchLoginProfileInput,
  WorkbenchOAuthBatchPreview,
} from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("登录资料在手机与桌面保持操作可见，替换后按版本预览重新授权", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const profile: WorkbenchLoginProfile = {
    id: "profile-a",
    account_id: "42",
    user_id: "user-a",
    email: "operator-" + "workspace".repeat(6) + "@example.test",
    workspace_id: "workspace-" + "long-id".repeat(16),
    revision: 4,
    updated_at: "2026-09-14T00:00:00Z",
    has_password: true,
    has_totp: true,
    has_proxy: true,
  };
  const saves: WorkbenchLoginProfileInput[] = [];
  const previews: unknown[] = [];
  const starts: unknown[] = [];
  await page.route("**/api/accounts", (route) =>
    route.fulfill({
      json: [
        {
          id: "43",
          name: "团队账号_" + "workspace".repeat(20),
          platform: "openai",
          account_type: "oauth",
        },
      ],
    }),
  );
  await page.route("**/api/account-workbench/login-profiles", async (route) => {
    if (route.request().method() === "POST") {
      saves.push(route.request().postDataJSON() as WorkbenchLoginProfileInput);
      profile.revision += 1;
      await route.fulfill({ json: profile });
    } else await route.fulfill({ json: [profile] });
  });
  await page.route("**/api/account-workbench/reauthorization/preview", async (route) => {
    previews.push(route.request().postDataJSON() as unknown);
    const preview: WorkbenchOAuthBatchPreview = {
      id: "reauth-preview",
      target: "https://sub2api.example.test",
      fresh_login: true,
      expires_at: new Date(Date.now() + 600000).toISOString(),
      items: [
        {
          index: 0,
          account_id: profile.account_id,
          user_id: profile.user_id,
          profile_id: profile.id,
          profile_revision: profile.revision,
          email: profile.email,
          workspace_id: profile.workspace_id,
          has_password: true,
          has_totp: false,
          status: "queued",
          message: "等待重新授权",
        },
      ],
      errors: [],
    };
    await route.fulfill({ json: preview });
  });
  await page.route("**/api/account-workbench/oauth-batches", async (route) => {
    starts.push(route.request().postDataJSON() as unknown);
    await route.fulfill({
      json: {
        id: "batch-a",
        task_id: "task-a",
        status: "queued",
        fresh_login: true,
        available: 0,
        message: "等待重新授权",
        expires_at: new Date(Date.now() + 600000).toISOString(),
        items: [],
      },
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  const mode = page.getByRole("tab", { name: "登录资料", exact: true });
  await mode.focus();
  await page.keyboard.press("Enter");
  await expect(mode).toHaveAttribute("aria-selected", "true");
  const panel = page.getByRole("region", { name: "登录资料", exact: true });
  await expect(panel.getByText(profile.email, { exact: true })).toBeVisible();
  await page.getByRole("combobox", { name: "绑定账号" }).click();
  await page.getByRole("option", { name: /团队账号_/ }).click();
  expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(page.getByRole("button", { name: "新增登录资料" })).toBeInViewport();
  await page.screenshot({
    path: test.info().outputPath("login-profiles-selection.png"),
    animations: "disabled",
  });
  await page.getByRole("button", { name: `替换 ${profile.email} 的登录资料` }).click();
  const editor = page.getByRole("dialog", { name: "替换登录资料", exact: true });
  await expect(editor.getByLabel("登录密码", { exact: true })).toHaveValue("");
  await expect(editor.getByLabel("登录代理", { exact: true })).toHaveValue("");
  await editor.getByLabel("登录密码", { exact: true }).fill("fixture-replacement-password");
  await editor
    .getByLabel("登录代理", { exact: true })
    .fill("https://user:fixture-proxy@proxy.example.test:443");
  await expect(editor.getByRole("button", { name: "保存登录资料", exact: true })).toBeInViewport();
  await editor.getByRole("button", { name: "保存登录资料", exact: true }).click();
  const confirm = page.getByRole("dialog", { name: "确认保存登录资料", exact: true });
  await expect(confirm).toContainText("完整替换原资料");
  expect(saves).toHaveLength(0);
  await page.screenshot({
    path: test.info().outputPath("login-profile-save-confirm.png"),
    animations: "disabled",
  });
  await confirm.getByRole("button", { name: "确认保存到服务器" }).click();
  await expect(editor).toHaveCount(0);
  expect(saves[0]).toMatchObject({
    id: "profile-a",
    account_id: "42",
    revision: 4,
    confirmed: true,
    login: {
      password: "fixture-replacement-password",
      proxy_url: "https://user:fixture-proxy@proxy.example.test:443",
    },
  });
  const choice = page.getByRole("checkbox", { name: `选择 ${profile.email}（ID 42）` });
  await choice.focus();
  await page.keyboard.press("Space");
  await expect(choice).toBeChecked();
  await page.getByRole("button", { name: "预览重新授权（1）" }).click();
  const preview = page.getByRole("region", { name: "批量授权预览" });
  await expect(preview).toContainText("资料版本：5");
  expect(previews).toEqual([{ account_ids: ["42"], fresh_login: true }]);
  await preview.getByRole("button", { name: "确认授权 1 个账号" }).scrollIntoViewIfNeeded();
  await expect(preview.getByRole("button", { name: "确认授权 1 个账号" })).toBeInViewport();
  expect(await preview.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await preview.getByRole("button", { name: "确认授权 1 个账号" }).click();
  await expect(page.getByRole("dialog", { name: "确认批量授权" })).toContainText(
    "全新的官方登录会话",
  );
  expect(starts).toHaveLength(0);
});
