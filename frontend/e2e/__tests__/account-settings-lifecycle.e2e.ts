import { expect, test } from "@playwright/test";
import { account, task } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("保存等待时锁定字段，关闭再打开不受旧任务响应影响", async ({ page }) => {
  let release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  let settingsWrites = 0;
  let cancelledTasks = 0;
  const row = { ...account, priority: 8000 };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/accounts/41/settings") {
      settingsWrites++;
      await pending;
      await route.fulfill({ json: task("late-settings") });
      return;
    }
    if (path.includes("/cancel")) cancelledTasks++;
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离编辑测试" },
      "/api/accounts": [row],
      "/api/accounts/41": {
        ...row,
        metadata: {},
        group_rates: {},
        group_ids: {},
        bindings: [],
        test_models: ["current-model"],
      },
      "/api/model-checks/account-statuses": [],
      "/api/groups": [],
      "/api/policy": { available: true, advanced_policy: {} },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in responses && route.request().method() === "GET")
      await route.fulfill({ json: responses[path] });
    else await route.fulfill({ status: 503, json: { detail: "未配置的隔离接口" } });
  });
  await page.goto("/accounts");
  async function openEditor(): Promise<void> {
    await page.getByRole("button", { name: "更多账号操作", exact: true }).click();
    await page.getByRole("menuitem", { name: "查看并编辑账号", exact: true }).click();
  }
  await openEditor();
  const dialog = page.getByRole("dialog", { name: "账号设置", exact: true });
  await dialog.getByRole("spinbutton", { name: "并发上限", exact: true }).fill("23");
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect.poll(() => settingsWrites).toBe(1);
  await expect(dialog.getByRole("spinbutton", { name: "并发上限", exact: true })).toBeDisabled();
  await expect(dialog.getByRole("textbox", { name: "探测模型", exact: true })).toBeDisabled();
  await expect(dialog.getByRole("switch", { name: "暂停调度", exact: true })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  await expect(dialog.getByRole("button", { name: "获取模型", exact: true })).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await openEditor();
  await dialog.getByRole("spinbutton", { name: "并发上限", exact: true }).fill("31");
  const response = page.waitForResponse(
    (value) => new URL(value.url()).pathname === "/api/accounts/41/settings",
  );
  release();
  await response;
  await expect(dialog.getByRole("spinbutton", { name: "并发上限", exact: true })).toHaveValue("31");
  await expect(dialog.getByRole("button", { name: "保存", exact: true })).toBeEnabled();
  await expect(page.getByText("账号设置已执行完成", { exact: true })).toHaveCount(0);
  expect(settingsWrites).toBe(1);
  expect(cancelledTasks).toBe(0);
});
