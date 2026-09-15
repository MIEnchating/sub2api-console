import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("无管理目标时从处理记录只显示本批私有文件，删除需确认且不请求线上模板", async ({ page }) => {
  await installWorkbenchFixture(page);
  const managed: string[] = [];
  page.on("request", (request) => {
    if (
      /\/api\/account-workbench\/(templates|exports|import)$/.test(new URL(request.url()).pathname)
    )
      managed.push(request.url());
  });
  await page.route("**/api/setup/status", (route) =>
    route.fulfill({
      json: { initialized: true, target_configured: false, configuration_errors: [] },
    }),
  );
  const task = {
    id: "local-file-task",
    skill: "account-workbench",
    operation: "account-workbench-convert",
    status: "succeeded",
    progress: 100,
    message: "本批文件已生成",
    created_at: "",
    updated_at: "",
    result: {
      items: [
        {
          index: 0,
          name: "本地账号",
          status: "succeeded",
          report: { artifact_id: "local-file", scope: "local-export" },
        },
      ],
    },
  };
  let deleted = false;
  await page.route("**/api/account-workbench/history", (route) => route.fulfill({ json: [task] }));
  await page.route("**/api/tasks/local-file-task", (route) => route.fulfill({ json: task }));
  await page.route("**/api/account-workbench/local-exports", (route) =>
    route.fulfill({
      json: [
        ...(deleted
          ? []
          : [{ id: "local-file", kind: "accounts", count: 1, expires_at: "2099-01-01T00:00:00Z" }]),
        { id: "unrelated-file", kind: "accounts", count: 2, expires_at: "2099-01-01T00:00:00Z" },
      ],
    }),
  );
  await page.route("**/api/account-workbench/local-exports/local-file", async (route) => {
    expect(route.request().method()).toBe("DELETE");
    deleted = true;
    await route.fulfill({ json: { deleted: true } });
  });
  await page.goto("/account-workbench");
  await expect(page.getByRole("button", { name: "仅导出 JSON" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await page.getByRole("tab", { name: "处理记录", exact: true }).click();
  await page.getByRole("button", { name: "查看任务 local-file-task" }).click();
  const files = page.getByRole("region", { name: "私有导出文件" });
  await expect(files.getByText("local-file", { exact: true })).toBeVisible();
  await expect(files).not.toContainText("unrelated-file");
  await expect(files.getByRole("button", { name: /再生|登录资料/ })).toHaveCount(0);
  await files.getByRole("button", { name: "删除私有文件 local-file" }).click();
  expect(deleted).toBe(false);
  const dialog = page.getByRole("dialog", { name: "删除私有文件" });
  await expect(dialog.getByRole("button", { name: "删除文件", exact: true })).toBeInViewport();
  await dialog.getByRole("button", { name: "删除文件", exact: true }).click();
  await expect(files.getByText("暂无私有导出文件")).toBeVisible();
  expect(managed).toEqual([]);
});

test("记录内继续授权接回原会话，离开详情不取消账号授权", async ({ page }) => {
  await installWorkbenchFixture(page);
  const task = {
    id: "oauth-fixture",
    skill: "account-workbench",
    operation: "account-workbench-oauth",
    status: "waiting_input",
    progress: 1,
    message: "需要账号验证",
    created_at: "",
    updated_at: "",
    result: {},
  };
  await page.route("**/api/account-workbench/history", (route) => route.fulfill({ json: [task] }));
  await page.route("**/api/tasks/oauth-fixture", (route) => route.fulfill({ json: task }));
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET" && request.url().includes("/oauth")) writes.push(request.url());
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "处理记录", exact: true }).click();
  await page.getByRole("button", { name: "查看任务 oauth-fixture" }).click();
  await page.getByRole("button", { name: "继续授权", exact: true }).click();
  await expect(page.getByRole("button", { name: "上游登录页面" })).toBeVisible();
  await page.getByRole("button", { name: "关闭详情" }).click();
  await expect(page.getByRole("button", { name: "上游登录页面" })).toHaveCount(0);
  expect(writes).toEqual([]);
});
