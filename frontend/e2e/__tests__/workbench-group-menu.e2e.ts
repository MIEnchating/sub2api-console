import { expect, test } from "@playwright/test";
import { accountListFixture } from "../../src/features/account-workbench/__tests__/list-pagination-fixture";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page }) => {
  const accounts = accountListFixture(1000);
  for (const account of accounts) account.groups = [{ id: account.id, name: `团队 ${account.id}` }];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "菜单验收" },
      "/api/dictionaries": { items: [] },
      "/api/account-workbench/accounts": accounts,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "账号列表", exact: true }).click();
  await expect(page.getByRole("button", { name: "查看 账号 1 配置", exact: true })).toBeVisible();
});

test("1000个分组首次展开只挂载可见选项，搜索仍可选择末项", async ({ page }) => {
  const start = await page.evaluate(() => performance.now());
  await page.getByRole("combobox", { name: "筛选账号分组" }).click();
  await expect(page.getByRole("option", { name: "全部分组", exact: true })).toBeVisible();
  const mounted = await page.getByRole("option").count();
  const elapsed = (await page.evaluate(() => performance.now())) - start;
  await test.info().attach("menu-open-measurement", {
    body: JSON.stringify({ mounted, elapsed }),
    contentType: "application/json",
  });
  expect(mounted).toBeLessThan(60);
  await page.getByLabel("搜索选项", { exact: true }).fill("1000");
  await page.getByRole("option", { name: "团队 1000", exact: true }).click();
  await expect(page.getByText("account-1000@example.test", { exact: true })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "筛选账号分组" })).toHaveText("团队 1000");
});

test("大目录键盘到达末项并选择，重新打开仍显示选中项", async ({ page }) => {
  const trigger = page.getByRole("combobox", { name: "筛选账号分组" });
  await trigger.focus();
  await page.keyboard.press("Enter");
  const search = page.getByLabel("搜索选项", { exact: true });
  await search.focus();
  await page.keyboard.press("ArrowUp");
  await page.keyboard.press("ArrowUp");
  await expect(page.getByRole("option", { name: "团队 1000", exact: true })).toBeInViewport();
  await page.keyboard.press("Enter");
  await expect(trigger).toHaveText("团队 1000");
  await trigger.click();
  await expect(page.getByRole("option", { name: "团队 1000", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("滚动可访问末项，清空搜索恢复完整列表，长名称不撑宽菜单", async ({ page }) => {
  const accounts = accountListFixture(1000);
  const longName = "长分组名称".repeat(35);
  for (const account of accounts) account.groups = [{ id: account.id, name: `团队 ${account.id}` }];
  accounts[999].groups[0].name = longName;
  await page.route("**/api/account-workbench/accounts", (route) =>
    route.fulfill({ json: accounts }),
  );
  await page.getByRole("button", { name: "刷新列表" }).click();
  await expect(page.getByRole("button", { name: "刷新列表" })).toBeEnabled();
  await page.getByRole("combobox", { name: "筛选账号分组" }).click();
  const list = page.getByRole("listbox");
  await list.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(page.getByRole("option", { name: longName, exact: true })).toBeVisible();
  expect(await list.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const search = page.getByLabel("搜索选项", { exact: true });
  await search.fill("不存在的分组");
  await expect(page.getByText("没有匹配的选项")).toBeVisible();
  await search.clear();
  await expect(page.getByRole("option", { name: "全部分组", exact: true })).toBeVisible();
  expect(await page.getByRole("option").count()).toBeLessThan(60);
  await search.fill("1000");
  await page.getByRole("option", { name: longName, exact: true }).click();
  await expect(page.getByText("account-1000@example.test", { exact: true })).toBeVisible();
  await page.getByRole("combobox", { name: "筛选账号分组" }).click();
  await search.fill("全部");
  await page.getByRole("option", { name: "全部分组", exact: true }).click();
  await expect(page.getByText("account-1@example.test", { exact: true })).toBeVisible();
});

test("大目录移除选中分组后保持范围，缩为小目录也可显式恢复全部", async ({ page }) => {
  const trigger = page.getByRole("combobox", { name: "筛选账号分组" });
  await trigger.click();
  await page.getByLabel("搜索选项", { exact: true }).fill("1000");
  await page.getByRole("option", { name: "团队 1000", exact: true }).click();
  const accounts = accountListFixture(101);
  for (const account of accounts) account.groups = [{ id: account.id, name: `团队 ${account.id}` }];
  await page.route("**/api/account-workbench/accounts", (route) =>
    route.fulfill({ json: accounts }),
  );
  await page.getByRole("button", { name: "刷新列表" }).click();
  await expect(page.getByText("没有匹配的账号")).toBeVisible();
  await expect(trigger).toContainText("分组 #1000");
  await page.route("**/api/account-workbench/accounts", (route) =>
    route.fulfill({ json: accounts.slice(0, 1) }),
  );
  await page.getByRole("button", { name: "刷新列表" }).click();
  await expect(page.getByRole("button", { name: "刷新列表" })).toBeEnabled();
  await expect(trigger).toContainText("分组 #1000");
  await trigger.click();
  await page.getByRole("option", { name: "全部分组", exact: true }).click();
  await expect(page.getByText("account-1@example.test", { exact: true })).toBeVisible();
});
