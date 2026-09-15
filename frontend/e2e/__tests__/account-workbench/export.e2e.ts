import { expect, test } from "@playwright/test";
import type { Task, WorkbenchExportPreview } from "../../../src/api";
import { pageFixtures } from "../fixtures/page-shell";

test("私有导出在桌面和手机按稳定 ID 确认范围且长账号名不撑开页面", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const longName = "团队账号_" + "Workspace".repeat(30);
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const task: Task = {
    id: "private-export-task",
    skill: "account-workbench",
    operation: "account-workbench-export",
    status: "queued",
    progress: 0,
    message: "等待生成私有文件",
    result: {},
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
  const preview: WorkbenchExportPreview = {
    id: "private-preview",
    revision: "preview-version",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    items: [{ account_id: "42", name: longName, revision: "account-version" }],
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const method = route.request().method();
    requests.push({
      path,
      method,
      body: route.request().postData() ? (route.request().postDataJSON() as unknown) : null,
    });
    if (path.endsWith("/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    if (path === "/api/account-workbench/exports/preview") return route.fulfill({ json: preview });
    if (path === "/api/account-workbench/exports" && method === "POST")
      return route.fulfill({ json: task });
    if (path === "/api/tasks/private-export-task") return route.fulfill({ json: task });
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "导出集成测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/account-workbench/history": [],
      "/api/account-workbench/templates": [],
      "/api/account-workbench/exports": [],
      "/api/accounts": [{ id: "42", name: longName, platform: "openai", account_type: "oauth" }],
    };
    if (path in responses) return route.fulfill({ json: responses[path] });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "私有导出", exact: true }).click();
  const account = page.getByRole("checkbox", { name: `${longName}（ID 42）` });
  await account.focus();
  await page.keyboard.press("Space");
  await expect(account).toBeChecked();
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.getByRole("button", { name: "预览导出范围" }).click();
  const scope = page.getByRole("region", { name: "账号导出预览" });
  await expect(scope).toBeVisible();
  await expect(scope.getByRole("list", { name: "导出账号范围" })).toContainText(longName);
  expect(await scope.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.getByRole("button", { name: "确认导出 1 个账号" }).click();
  const dialog = page.getByRole("dialog", { name: "确认生成私有账号文件" });
  await expect(dialog).toContainText("ID：42");
  await expect(dialog).toContainText("后端私有目录");
  expect(requests.some((item) => item.path.endsWith("/exports") && item.method === "POST")).toBe(
    false,
  );
  await page.screenshot({
    path: test.info().outputPath("private-export-confirm.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "创建导出任务" }).click();
  await expect
    .poll(
      () => requests.find((item) => item.path.endsWith("/exports") && item.method === "POST")?.body,
    )
    .toEqual({ preview_id: "private-preview", confirmed: true });
  await expect(page.getByRole("status", { name: "等待生成私有文件" })).toBeVisible();
  await expect(page.getByRole("progressbar")).toHaveCount(0);
});
