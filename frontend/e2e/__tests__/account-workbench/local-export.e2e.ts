import { expect, test } from "@playwright/test";
import type { Task, WorkbenchPreview } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("无管理目标时默认本地导出，长账号预览及私有文件删除在手机和桌面均可操作", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const managedRequests: string[] = [];
  const previews: unknown[] = [];
  const conversions: unknown[] = [];
  let removed = false;
  page.on("request", (request) => {
    if (
      /\/api\/account-workbench\/(templates|import|exports)$/.test(new URL(request.url()).pathname)
    )
      managedRequests.push(request.url());
  });
  await page.route("**/api/setup/status", (route) =>
    route.fulfill({
      json: { initialized: true, target_configured: false, configuration_errors: [] },
    }),
  );
  const task: Task = {
    id: "local-conversion",
    skill: "account-workbench",
    operation: "account-workbench-convert",
    status: "queued",
    progress: 0,
    message: "等待生成私有账号文件",
    result: {},
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
  };
  await page.route("**/api/tasks/local-conversion", (route) => route.fulfill({ json: task }));
  await page.route("**/api/account-workbench/exports/from-input", async (route) => {
    conversions.push(route.request().postDataJSON() as unknown);
    await route.fulfill({ json: task });
  });
  await page.route("**/api/account-workbench/preview", async (route) => {
    previews.push(route.request().postDataJSON() as unknown);
    const preview: WorkbenchPreview = {
      id: "local-preview",
      scope: "local-export",
      export_only: true,
      target: "",
      expires_at: new Date(Date.now() + 600000).toISOString(),
      check_after_import: false,
      model: "",
      errors: [],
      items: [
        {
          id: "0",
          index: 0,
          name: "独立账号_" + "local-account".repeat(20),
          email: "local@example.test",
          plan_type: "plus",
          template_id: "",
          template_name: "",
          template_revision: 0,
          group_ids: [],
          duplicate: false,
        },
      ],
    };
    await route.fulfill({ json: preview });
  });
  await page.route("**/api/account-workbench/local-exports", (route) =>
    route.fulfill({
      json: removed
        ? []
        : [
            {
              id: "local-private-file",
              kind: "accounts",
              count: 1,
              created_at: "2026-09-14T00:00:00Z",
              expires_at: "2026-09-15T00:00:00Z",
            },
          ],
    }),
  );
  await page.route("**/api/account-workbench/local-exports/local-private-file", async (route) => {
    expect(route.request().method()).toBe("DELETE");
    removed = true;
    await route.fulfill({ json: { deleted: true } });
  });
  await page.goto("/account-workbench");
  await expect(page.getByRole("tab", { name: "本地导出", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByRole("combobox", { name: "配置模板" })).toHaveCount(0);
  const input = page.getByRole("textbox", { name: "账号内容" });
  const content =
    '{"credentials":{"access_token":"isolated-local-token","chatgpt_user_id":"local-user","chatgpt_account_id":"local-workspace"}}';
  await input.fill(content);
  await page.getByRole("button", { name: "解析并预览" }).click();
  await expect(page.getByRole("table", { name: "账号预览" })).toBeVisible();
  expect(previews).toEqual([
    { content, scope: "local-export", export_only: true, check_after_import: false, model: "" },
  ]);
  await page.getByRole("button", { name: "生成私有 JSON 文件" }).click();
  const dialog = page.getByRole("dialog", { name: "确认生成私有账号文件" });
  await expect(dialog.getByRole("button", { name: "创建私有转换任务" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(
    await page
      .locator('[data-slot="page-content"]')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("local-export-confirm.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "创建私有转换任务" }).click();
  await expect(input).toHaveText("");
  expect(conversions).toEqual([{ preview_id: "local-preview", confirmed: true }]);
  const files = page.getByRole("tab", { name: "私有文件", exact: true });
  await files.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByText("local-private-file", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "删除私有文件 local-private-file" }).click();
  expect(removed).toBe(false);
  await page
    .getByRole("dialog", { name: "删除私有文件" })
    .getByRole("button", { name: "删除文件" })
    .click();
  await expect(page.getByText("暂无私有导出文件")).toBeVisible();
  expect(managedRequests).toEqual([]);
  expect(
    await page.evaluate(() =>
      Object.keys(localStorage).every((key) => !/token|credential|workbench/i.test(key)),
    ),
  ).toBe(true);
});

test("初始化可通过键盘选择本地模式并且不提交隐藏的管理凭据", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  let initialized = false;
  const requests: unknown[] = [];
  await page.route("**/api/setup/status", (route) =>
    route.fulfill({
      json: {
        initialized,
        target_configured: false,
        setup_token_required: false,
        configuration_errors: [],
      },
    }),
  );
  await page.route("**/api/setup/initialize", async (route) => {
    requests.push(route.request().postDataJSON() as unknown);
    initialized = true;
    await route.fulfill({ json: { initialized: true, target_configured: false } });
  });
  await page.goto("/account-workbench");
  await page.getByLabel("控制台账号", { exact: true }).fill("local-operator");
  await page.getByLabel("控制台密码", { exact: true }).fill("isolated-local-password");
  await page.getByLabel("确认控制台密码", { exact: true }).fill("isolated-local-password");
  await page.getByLabel("Admin Base URL", { exact: true }).fill("unfinished-target");
  await page.getByLabel("Admin Key", { exact: true }).fill("isolated-managed-key");
  const local = page.getByRole("checkbox", { name: "仅使用本地账号工作台（不配置线上管理目标）" });
  await local.focus();
  await page.keyboard.press("Space");
  await expect(local).toBeChecked();
  await expect(page.getByLabel("Admin Base URL", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "完成初始化" }).scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("local-export-setup.png"),
    animations: "disabled",
  });
  await page.getByRole("button", { name: "完成初始化" }).click();
  await expect(page.getByRole("tab", { name: "本地导出", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  expect(requests).toEqual([
    {
      username: "local-operator",
      password: "isolated-local-password",
      local_export_only: true,
      admin_base_url: "",
      admin_key: "",
    },
  ]);
});
