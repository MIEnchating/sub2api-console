import { expect, test } from "@playwright/test";
import type {
  WorkbenchOAuthSession,
  WorkbenchSecurityBatch,
  WorkbenchSecurityBatchPreview,
} from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("多个本地检查点完成批量安全设置后，显式发起新授权并关闭来源弹窗", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const expires = new Date(Date.now() + 600000).toISOString();
  const checkpoints = [
    { id: "checkpoint-1", source_task_id: "original-1", revision: 5, checkpoint_revision: 7 },
    { id: "checkpoint-2", source_task_id: "original-2", revision: 8, checkpoint_revision: 9 },
  ].map((item) => ({
    ...item,
    scope: "local-export",
    status: "ready",
    stage: "phone",
    can_restore: true,
    created_at: expires,
    expires_at: expires,
  }));
  let identityConfirmed = false;
  const writes: Array<{ path: string; body: unknown }> = [];
  const identity = {
    email: "official@example.test",
    user_id: "user-" + "long-official-user".repeat(18),
  };
  const items: WorkbenchSecurityBatchPreview["items"] = checkpoints.map((item, index) => ({
    index,
    source: {
      checkpoint_id: item.id,
      revision: item.revision,
      checkpoint_revision: item.checkpoint_revision,
    },
    email: "",
    user_id: "",
    status: "queued",
    message: "待确认官方身份",
  }));
  const preview: WorkbenchSecurityBatchPreview = {
    id: "source-batch-preview",
    scope: "local-export",
    target: "",
    operation: "totp",
    expires_at: expires,
    items,
    errors: [],
  };
  let batch: WorkbenchSecurityBatch = {
    ...preview,
    id: "source-batch",
    task_id: "source-batch",
    status: "running",
    message: "等待当前账号身份确认",
    completed: 0,
    succeeded: 0,
    current_security_id: "security-child",
  };
  const freshOAuth: WorkbenchOAuthSession = {
    id: "fresh-batch-oauth",
    task_id: "fresh-batch-oauth",
    scope: "local-export",
    host: "auth.openai.com",
    status: "waiting",
    message: "批量安全后的新授权已建立",
    expires_at: expires,
    width: 1100,
    height: 760,
  };
  await page.route("**/api/account-workbench/oauth-checkpoints?scope=local-export", (route) =>
    route.fulfill({ json: checkpoints }),
  );
  await page.route("**/api/account-workbench/security-batches/preview", (route) => {
    writes.push({ path: "preview", body: route.request().postDataJSON() as unknown });
    return route.fulfill({ json: preview });
  });
  await page.route("**/api/account-workbench/security-batches", (route) => {
    writes.push({ path: "start", body: route.request().postDataJSON() as unknown });
    return route.fulfill({ json: batch });
  });
  await page.route("**/api/account-workbench/security-batches/source-batch", (route) =>
    route.fulfill({ json: route.request().method() === "DELETE" ? { cancelled: true } : batch }),
  );
  await page.route("**/api/account-workbench/security/security-child", (route) => {
    let status = "awaiting_confirmation";
    if (identityConfirmed) status = "waiting";
    if (batch.status === "partial") status = "succeeded";
    return route.fulfill({
      json: {
        id: "security-child",
        task_id: "security-child",
        source_checkpoint_id: "checkpoint-1",
        status,
        scope: "local-export",
        operation: "totp",
        ...identity,
        message: "请确认本批当前账号",
        expires_at: expires,
        width: 1100,
        height: 760,
      },
    });
  });
  await page.route("**/api/account-workbench/security/security-child/confirm-identity", (route) => {
    writes.push({ path: "identity", body: route.request().postDataJSON() as unknown });
    identityConfirmed = true;
    return route.fulfill({ json: { accepted: true } });
  });
  await page.route("**/api/account-workbench/security/security-child/continue", (route) => {
    batch = {
      ...batch,
      status: "partial",
      message: "批量安全设置已结束",
      current_security_id: undefined,
      completed: 2,
      succeeded: 1,
      items: items.map((item) => {
        if (item.index === 0)
          return {
            ...item,
            ...identity,
            status: "succeeded",
            security_id: "security-child",
            message: "双重验证已启用",
          };
        return { ...item, status: "failed", message: "官方身份验证未完成" };
      }),
    };
    return route.fulfill({ json: { accepted: true } });
  });
  await page.route("**/api/account-workbench/security/security-child/oauth", (route) => {
    writes.push({ path: "security-child/oauth", body: route.request().postDataJSON() as unknown });
    return route.fulfill({ json: freshOAuth });
  });
  await page.route("**/api/account-workbench/oauth/fresh-batch-oauth", (route) =>
    route.fulfill({ json: freshOAuth }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "本地导出", exact: true }).click();
  await page.getByRole("tab", { name: "单个授权", exact: true }).click();
  await page.getByRole("button", { name: "已暂停的授权" }).click();
  await page.getByRole("button", { name: "批量设置检查点账号安全" }).click();
  const dialog = page.getByRole("dialog", { name: "授权来源批量安全设置" });
  await dialog.getByRole("checkbox", { name: "来源任务：original-1", exact: true }).check();
  await dialog.getByRole("checkbox", { name: "来源任务：original-2", exact: true }).check();
  await dialog.getByRole("button", { name: "预览批量安全操作" }).click();
  await expect(dialog.getByRole("region", { name: "批量安全操作预览" })).toContainText(
    "待确认官方身份",
  );
  expect(writes).toHaveLength(1);
  expect(writes[0]?.body).toEqual({
    scope: "local-export",
    sources: items.map((item) => item.source),
    operation: "totp",
  });
  await dialog.getByRole("button", { name: "确认并执行批量安全操作" }).click();
  await expect(dialog.getByRole("button", { name: "验证完成，继续当前账号" })).toBeDisabled();
  const confirm = dialog.getByRole("button", { name: "确认当前官方账号" });
  await confirm.scrollIntoViewIfNeeded();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "返回授权来源" })).toBeInViewport();
  await page.screenshot({
    path: test.info().outputPath("source-batch-identity.png"),
    animations: "disabled",
  });
  await confirm.click();
  await expect(dialog.getByRole("button", { name: "验证完成，继续当前账号" })).toBeEnabled();
  await expect(dialog.getByRole("button", { name: "确认发起新的 OAuth 授权" })).toHaveCount(0);
  expect(writes[1]).toEqual({
    path: "start",
    body: { preview_id: "source-batch-preview", confirmed: true },
  });
  expect(writes.find((row) => row.path === "identity")?.body).toEqual({
    ...identity,
    confirmed: true,
  });
  await dialog.getByRole("button", { name: "验证完成，继续当前账号" }).click();
  await expect(dialog.getByRole("region", { name: "批量安全操作结果" })).toContainText(
    "已处理 2/2，成功 1",
  );
  const authorize = dialog.getByRole("button", { name: "确认发起新的 OAuth 授权" });
  await expect(authorize).toHaveCount(1);
  await authorize.scrollIntoViewIfNeeded();
  await expect(authorize).toBeInViewport();
  expect(writes.filter((row) => row.path === "security-child/oauth")).toEqual([]);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("source-batch-completed.png"),
    animations: "disabled",
  });
  await authorize.click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("dialog", { name: "已暂停的授权", exact: true })).toHaveCount(0);
  await expect(page.getByText(freshOAuth.message, { exact: true })).toBeVisible();
  expect(writes.filter((row) => row.path === "security-child/oauth")).toEqual([
    { path: "security-child/oauth", body: { confirmed: true } },
  ]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("source-batch-fresh-oauth.png"),
    animations: "disabled",
  });
});
