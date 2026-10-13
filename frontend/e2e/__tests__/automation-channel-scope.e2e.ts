import { expect, test } from "@playwright/test";
import type { AccountModelAutomationSchedule, NewAPIChannel } from "../../src/api";
import { pageFixtures, workspace } from "./fixtures/page-shell";

test("缺失已选渠道保留ID，键盘移除后保存明确范围且手机不横向溢出", async ({ page }) => {
  const channel: NewAPIChannel = {
    id: "11",
    name: "生产渠道名称较长时仍保留完整选择状态",
    type: 1,
    status: 1,
    models: ["model-a"],
    groups: [],
    version: "a".repeat(64),
  };
  const schedule: AccountModelAutomationSchedule = {
    id: "scope-test",
    name: "渠道范围保护",
    enabled: true,
    interval_minutes: 60,
    last_run_at: null,
    next_run_at: null,
    request: {
      platform_id: "layout",
      group_ids: ["7"],
      selected_models: ["model-a"],
      channels: [
        { id: channel.id, version: channel.version },
        { id: "99", version: "b".repeat(64) },
      ],
      publish_missing: true,
      price_enabled: true,
      pricing_source: "official",
    },
  };
  let submitted: unknown;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/channels/lookup")) {
      await route.fulfill({
        json: {
          items: [channel],
          failures: [{ id: "99", code: "channel_unavailable", message: "渠道读取失败" }],
        },
      });
      return;
    }
    if (path.endsWith("/channels")) {
      await route.fulfill({ json: { items: [channel], total: 1 } });
      return;
    }
    if (path.endsWith("/automation/schedules") && route.request().method() === "PUT") {
      submitted = route.request().postDataJSON();
      await route.fulfill({ json: schedule });
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
      "/api/catalog/group-memberships": [{ id: "7", name: "测试分组", account_count: 1 }],
      "/api/management/accounts/models/automation/schedules": [schedule],
      "/api/management/accounts/models/automation/task-groups": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/newapi/account-model-automation");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑运营任务" });
  const selection = dialog.getByRole("group", { name: "目标渠道", exact: true });
  await expect(selection).toHaveAttribute("aria-invalid", "true");
  await expect(dialog.getByRole("button", { name: "保存任务", exact: true })).toBeDisabled();
  await expect(dialog.getByRole("button", { name: "预览", exact: true })).toBeDisabled();
  const remove = selection.getByRole("button", { name: "移除未读取到的渠道 99" });
  await remove.focus();
  await expect(remove).toBeFocused();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("missing-channel.png") });
  await page.keyboard.press("Enter");
  await expect(remove).toHaveCount(0);
  await expect(dialog.getByRole("checkbox", { name: /生产渠道名称较长/ })).toBeChecked();
  await dialog.getByRole("button", { name: "保存任务", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(submitted).toMatchObject({ channels: [{ id: channel.id, version: channel.version }] });
});
