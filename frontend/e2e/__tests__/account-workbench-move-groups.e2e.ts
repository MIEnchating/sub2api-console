import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("账号操作移动分组支持窄屏、长文本、目标选择及完成刷新", async ({ page }, info) => {
  if (info.project.name === "mobile-dark") await page.setViewportSize({ width: 320, height: 600 });
  const name = "长名称账号".repeat(8);
  const groupName = "目标分组长名称".repeat(8);
  let moved = false;
  const writes: unknown[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const account = {
      id: "41",
      name,
      email: "owner@example.test",
      status: "active",
      schedulable: true,
      plan: "plus",
      groups: moved ? [{ id: "8", name: groupName }] : [{ id: "7", name: "原组" }],
      proxy_name: "",
      concurrency: "0",
      load_factor: "",
      rate_multiplier: "1",
      model_mapping: {},
      fingerprint: "",
    };
    if (path === "/api/account-workbench/accounts/41/groups") {
      if (route.request().method() === "PUT") {
        writes.push(route.request().postDataJSON());
        await route.fulfill({ json: { id: "move-task", status: "queued", result: {} } });
      } else {
        await route.fulfill({
          json: {
            account_id: "41",
            current_group_ids: ["7"],
            target_version: "target",
            account_version: "account",
            groups: [
              { id: "7", name: "原组" },
              { id: "8", name: groupName },
            ],
          },
        });
      }
      return;
    }
    if (path === "/api/tasks/move-task") {
      moved = true;
      await route.fulfill({ json: { id: "move-task", status: "succeeded", result: {} } });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "移动分组验收" },
      "/api/dictionaries": { items: [] },
      "/api/account-workbench/accounts": [account],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "账号列表", exact: true }).click();
  await page.getByRole("button", { name: `移动 ${name} 分组` }).click();
  const dialog = page.getByRole("dialog", { name: "移动分组" });
  const submit = dialog.getByRole("button", { name: "确认移动" });
  await expect(submit).toBeDisabled();
  await dialog.getByRole("combobox", { name: "目标分组" }).click();
  await page.getByRole("option", { name: "原组（#7）" }).click();
  await page.getByRole("option", { name: `${groupName}（#8）` }).click();
  await page.keyboard.press("Escape");
  await expect(submit).toBeEnabled();
  await expect(submit).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "取消" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("移动分组.png"), fullPage: true });
  await submit.click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("table", { name: "账号列表" }).getByText(groupName, { exact: true }),
  ).toBeVisible();
  expect(writes).toEqual([
    {
      group_ids: ["8"],
      expected_group_ids: ["7"],
      target_version: "target",
      account_version: "account",
    },
  ]);
});
