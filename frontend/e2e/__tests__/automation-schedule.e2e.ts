import { expect, test } from "@playwright/test";
import type { AccountModelAutomationSchedule, Task } from "../../src/api";
import { pageFixtures, workspace } from "./fixtures/page-shell";

test("未选渠道也可读取模型，指定每日多时间保存后列表和编辑回填一致", async ({ page }) => {
  let schedules: AccountModelAutomationSchedule[] = [];
  const discovery: Task = {
    id: "read-models",
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status: "succeeded",
    progress: 100,
    message: "读取完成",
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:00Z",
    result: { dry_run: true, models: ["model-a", "model-b"], items: [] },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/management/accounts/models/automation/schedules") {
      if (route.request().method() === "PUT") {
        const input = route.request().postDataJSON();
        expect(input).toMatchObject({
          schedule_type: "daily",
          daily_times: ["09:00", "18:30"],
          timezone: "Asia/Shanghai",
          channels: [],
          price_enabled: false,
          selected_models: ["model-b"],
        });
        schedules = [
          {
            ...input,
            id: "daily-1",
            request: input,
            last_run_at: null,
            next_run_at: "2026-10-09T01:00:00Z",
          },
        ];
        await route.fulfill({ json: schedules[0] });
      } else await route.fulfill({ json: schedules });
      return;
    }
    if (path === "/api/management/accounts/models/automation") {
      expect(route.request().postDataJSON()).toMatchObject({
        channels: [],
        dry_run: true,
        price_enabled: false,
      });
      await route.fulfill({ json: discovery });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离测试" },
      "/api/newapi": {
        ...workspace,
        bindings: [
          {
            platform_id: "layout",
            sub2api_group_id: "7",
            newapi_group_id: "7",
            newapi_group_name: "测试分组",
            sync_ratio: true,
          },
        ],
      },
      "/api/groups": [{ id: "7", name: "测试分组", account_count: 1 }],
      "/api/tasks/read-models": discovery,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/newapi/account-model-automation");
  await page.getByRole("button", { name: "添加任务", exact: true }).click();
  const editor = page.getByRole("dialog", { name: "添加运营任务" });
  await expect(editor.getByRole("button", { name: "读取可用模型" })).toBeDisabled();
  await editor.getByRole("textbox", { name: "运营任务名称" }).fill("每天模型同步");
  await editor.getByRole("checkbox", { name: /测试分组/ }).check();
  await editor.getByRole("button", { name: "读取可用模型" }).click();
  await expect(editor.getByText("已读取 2 个可用模型 · 已选择 0 个上架模型")).toBeVisible();
  await editor.getByRole("combobox", { name: "要上架的模型" }).click();
  await page.getByRole("option", { name: "model-b", exact: true }).click();
  await page.keyboard.press("Escape");
  await editor.getByRole("radio", { name: "指定时段" }).check();
  await editor.getByRole("button", { name: "添加时间" }).click();
  await editor.getByLabel("每天运行时间 2", { exact: true }).fill("18:30");
  expect(await editor.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await editor.getByRole("button", { name: "保存任务", exact: true }).click();
  await expect(editor).toHaveCount(0);
  await expect(page.getByText("每天 09:00、18:30（北京时间）", { exact: true })).toBeVisible();
  await page
    .getByRole("group", { name: "每天模型同步 的操作" })
    .getByRole("button", { name: "编辑", exact: true })
    .click();
  const editing = page.getByRole("dialog", { name: "编辑运营任务" });
  await expect(editing.getByRole("radio", { name: "指定时段" })).toBeChecked();
  await expect(editing.getByLabel("每天运行时间 2", { exact: true })).toHaveValue("18:30");
  await expect(editing.getByRole("button", { name: "读取可用模型" })).toBeEnabled();
});
