import { expect, test } from "@playwright/test";
import type {
  WorkbenchOAuthBatchPreview,
  WorkbenchSourceProfile,
  WorkbenchSourceProfileIdentity,
} from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("本地文件保存登录资料后可按稳定版本私有导出和重新登录，桌面及手机均可确认与取消", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const profile: WorkbenchSourceProfile = {
    id: "local-profile-a",
    scope: "local-export",
    revision: 1,
    email: "local.operator@example.test",
    user_id: "official-user-" + "identity".repeat(15),
    workspace_id: "workspace-" + "isolated".repeat(15),
    has_password: true,
    has_totp: false,
    has_proxy: false,
    updated_at: "2026-09-14T00:00:00Z",
  };
  const source = { artifact_id: "local-account-file", index: 1 };
  const identity: WorkbenchSourceProfileIdentity = {
    scope: "local-export",
    source,
    source_revision: "a".repeat(64),
    email: profile.email,
    user_id: profile.user_id,
    workspace_id: profile.workspace_id,
    expires_at: new Date(Date.now() + 600000).toISOString(),
  };
  let saved = false;
  const saves: unknown[] = [],
    identities: unknown[] = [],
    exports: unknown[] = [],
    authorizations: unknown[] = [];
  const forbidden: string[] = [];
  page.on("request", (request) => {
    if (
      /\/api\/account-workbench\/(templates|login-profiles|reauthorization|exports\/profiles|import)(\/|$)/.test(
        new URL(request.url()).pathname,
      )
    )
      forbidden.push(request.url());
  });
  await page.route("**/api/setup/status", (route) =>
    route.fulfill({
      json: { initialized: true, target_configured: false, configuration_errors: [] },
    }),
  );
  await page.route("**/api/account-workbench/local-exports", (route) =>
    route.fulfill({
      json: [
        {
          id: source.artifact_id,
          kind: "accounts",
          count: 2,
          created_at: "2026-09-14T00:00:00Z",
          expires_at: identity.expires_at,
        },
      ],
    }),
  );
  await page.route("**/api/account-workbench/source-profiles/source", async (route) => {
    identities.push(route.request().postDataJSON() as unknown);
    await route.fulfill({ json: identity });
  });
  await page.route(/\/api\/account-workbench\/source-profiles(?:\?.*)?$/, async (route) => {
    if (route.request().method() === "POST") {
      saves.push(route.request().postDataJSON() as unknown);
      saved = true;
      await route.fulfill({ json: profile });
    } else {
      expect(new URL(route.request().url()).searchParams.get("scope")).toBe("local-export");
      await route.fulfill({ json: saved ? [profile] : [] });
    }
  });
  await page.route("**/api/account-workbench/source-profiles/exports/preview", async (route) => {
    exports.push(route.request().postDataJSON() as unknown);
    await route.fulfill({
      json: {
        id: "local-profile-export-preview",
        scope: "local-export",
        kind: "login-profiles",
        target: "",
        expires_at: identity.expires_at,
        items: [profile],
      },
    });
  });
  await page.route(
    "**/api/account-workbench/exports/preview/local-profile-export-preview",
    (route) => route.fulfill({ json: { deleted: true } }),
  );
  await page.route(
    "**/api/account-workbench/source-profiles/reauthorization/preview",
    async (route) => {
      authorizations.push(route.request().postDataJSON() as unknown);
      const preview: WorkbenchOAuthBatchPreview = {
        id: "local-profile-login-preview",
        scope: "local-export",
        fresh_login: true,
        target: "",
        expires_at: identity.expires_at,
        errors: [],
        items: [
          {
            index: 0,
            email: profile.email,
            user_id: profile.user_id,
            workspace_id: profile.workspace_id,
            profile_id: profile.id,
            profile_revision: profile.revision,
            has_password: true,
            has_totp: false,
            status: "queued",
            message: "等待重新登录",
          },
        ],
      };
      await route.fulfill({ json: preview });
    },
  );
  await page.route(
    "**/api/account-workbench/oauth-batches/preview/local-profile-login-preview",
    (route) => route.fulfill({ json: { deleted: true } }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "私有文件", exact: true }).click();
  await page.getByRole("button", { name: `创建文件登录资料 ${source.artifact_id}` }).click();
  await page.getByRole("spinbutton", { name: "账号序号" }).fill("2");
  await page.getByRole("button", { name: "核对所选账号身份" }).click();
  const editor = page.getByRole("dialog", { name: "新增登录资料", exact: true });
  await expect(editor.getByLabel("登录邮箱", { exact: true })).toHaveValue(profile.email);
  await expect(editor.getByLabel("登录邮箱", { exact: true })).toHaveAttribute("readonly", "");
  await editor.getByLabel("登录密码", { exact: true }).fill("isolated-source-password");
  await editor.getByRole("button", { name: "保存登录资料" }).click();
  const confirmation = page.getByRole("dialog", { name: "确认保存登录资料" });
  await expect(confirmation.getByRole("button", { name: "确认保存到服务器" })).toBeInViewport();
  expect(saves).toEqual([]);
  expect(await confirmation.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("local-source-profile-confirm.png"),
    animations: "disabled",
  });
  await confirmation.getByRole("button", { name: "确认保存到服务器" }).click();
  await expect(editor).toHaveCount(0);
  expect(identities.length).toBeGreaterThan(0);
  for (const input of identities) expect(input).toEqual({ scope: "local-export", source });
  expect(saves).toEqual([
    {
      scope: "local-export",
      source,
      source_revision: identity.source_revision,
      confirmed: true,
      login: {
        email: profile.email,
        workspace_id: profile.workspace_id,
        password: "isolated-source-password",
      },
    },
  ]);
  await page.getByRole("tab", { name: "登录资料", exact: true }).click();
  const list = page.getByRole("list", { name: "已保存本地登录资料" });
  await expect(list).toContainText(profile.email);
  const checkbox = page.getByRole("checkbox", { name: `选择本地资料 ${profile.email}` });
  await checkbox.focus();
  await page.keyboard.press("Space");
  await expect(checkbox).toBeChecked();
  await page.getByRole("button", { name: "私有导出资料（1）" }).click();
  const exported = page.getByRole("dialog", { name: "确认导出登录资料" });
  await expect(exported.getByRole("list", { name: "资料导出范围" })).toContainText(profile.email);
  await expect(exported).toContainText("范围：本地登录资料");
  await expect(exported.getByRole("button", { name: "确认生成资料文件" })).toBeInViewport();
  expect(exports).toEqual([{ scope: "local-export", items: [{ id: profile.id, revision: 1 }] }]);
  await exported.getByRole("button", { name: "返回", exact: true }).click();
  await page.getByRole("button", { name: "预览重新登录（1）" }).click();
  await page.getByRole("button", { name: "确认授权 1 个账号" }).click();
  const login = page.getByRole("dialog", { name: "确认批量授权" });
  await expect(login).toContainText("本地私有导出");
  await expect(login.getByRole("button", { name: "开始批量授权" })).toBeInViewport();
  expect(await login.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("local-source-profile-login.png"),
    animations: "disabled",
  });
  await login.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("button", { name: "关闭授权预览" }).click();
  await expect(page.getByRole("list", { name: "已保存本地登录资料" })).toContainText(profile.email);
  expect(authorizations).toEqual([
    { scope: "local-export", items: [{ id: profile.id, revision: 1 }], fresh_login: true },
  ]);
  expect(forbidden).toEqual([]);
  expect(
    await page
      .locator('[data-slot="page-content"]')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  expect(await page.evaluate(() => JSON.stringify({ ...localStorage }))).not.toMatch(
    /isolated-source-password|source_revision|local-profile-a/,
  );
});
