import { expect, test } from "@playwright/test";
import {
  accountListFixture,
  runItemsFixture,
} from "../../src/features/account-workbench/__tests__/list-pagination-fixture";
import { pageFixtures } from "./fixtures/page-shell";

for (const compact of [false, true]) {
  test(`${compact ? "320px" : "默认视口"}账号与处理结果分页保持全量搜索及正确启用范围`, async ({
    page,
  }, info) => {
    if (compact) await page.setViewportSize({ width: 320, height: 568 });
    const errors: Error[] = [];
    page.on("pageerror", (error) => errors.push(error));
    const accounts = accountListFixture(500);
    const run = runItemsFixture(500);
    accounts[0].usage = {
      five_hour: { used_percent: 0 },
      seven_day: { used_percent: 62.5 },
      updated_at: "2026-10-12T00:00:00Z",
    };
    const longEmail = `${"long-account-".repeat(24)}@example.test`;
    accounts[0].email = longEmail;
    run.items[0].email = longEmail;
    const writes: unknown[] = [];
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path.endsWith("/enable")) {
        writes.push(route.request().postDataJSON());
        await route.fulfill({ json: {} });
        return;
      }
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "工作台分页验收" },
        "/api/dictionaries": { items: [] },
        "/api/account-workbench/templates": { revision: 0, preferred_id: "", items: [] },
        "/api/account-workbench/accounts": accounts,
        "/api/account-workbench/runs": [run],
      };
      if (path.endsWith("/events")) {
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      } else if (path in fixtures) {
        await route.fulfill({ json: fixtures[path] });
      } else {
        await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      }
    });
    await page.goto("/account-workbench");
    await page.getByRole("tab", { name: "账号列表", exact: true }).click();
    const accountSection = page.getByRole("region", { name: "线上账号列表" });
    const accountTable = accountSection.getByRole("table", { name: "账号列表" });
    await expect(accountTable.getByRole("row")).toHaveCount(21);
    await expect(accountTable.getByRole("meter", { name: "5h 已用" })).toHaveAttribute(
      "aria-valuenow",
      "0",
    );
    await expect(accountTable.getByRole("meter", { name: "7d 已用" })).toHaveAttribute(
      "aria-valuenow",
      "62.5",
    );
    await expect(
      accountSection.getByRole("group", { name: "工作台操作", exact: true }),
    ).not.toContainText("个账号");

    await expect(accountTable.getByRole("cell", { name: longEmail, exact: false })).toBeVisible();
    expect(
      await page.locator("body").evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
    const accountContainer = accountSection.locator('[data-slot="table-container"]');
    expect(await accountContainer.evaluate((element) => getComputedStyle(element).overflowX)).toBe(
      "auto",
    );
    if (compact || info.project.name === "mobile-dark") {
      expect(
        await accountContainer.evaluate((element) => element.scrollWidth > element.clientWidth),
      ).toBe(true);
    }
    await accountSection.getByRole("button", { name: "转到下一页" }).focus();
    await page.keyboard.press("Enter");
    await expect(accountTable.getByText("account-21@example.test", { exact: true })).toBeVisible();
    await accountSection.getByRole("button", { name: "查看 账号 21 配置" }).click();
    await expect(page.getByRole("dialog")).toContainText("账号 #21 · account-21@example.test");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await accountSection.getByRole("searchbox", { name: "搜索账号" }).fill("account-500@");
    await expect(accountTable.getByRole("row")).toHaveCount(2);
    await expect(accountTable.getByText("account-500@example.test", { exact: true })).toBeVisible();
    await accountSection.getByRole("searchbox", { name: "搜索账号" }).fill("");
    await expect(accountSection.getByRole("button", { name: "转到上一页" })).toBeDisabled();

    await page.getByRole("tab", { name: "处理记录", exact: true }).click();
    const results = page.getByRole("region", { name: "处理记录" });
    const resultTable = results.getByRole("table", { name: "账号处理结果" });
    await results.getByRole("tab", { name: "已完成", exact: true }).click();
    await expect(results.getByText("没有匹配的处理记录")).toBeVisible();
    await results.getByRole("tab", { name: "需要处理", exact: true }).click();
    await expect(results.getByRole("tab", { name: "需要处理", exact: true })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await expect(resultTable.getByRole("row")).toHaveCount(21);
    expect(
      await page.locator("body").evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
    await results.getByRole("button", { name: "转到下一页" }).click();
    const row = resultTable.getByRole("row").filter({ hasText: "account-21@example.test" });
    await expect(row).toBeVisible();
    run.revision = 2;
    await results.getByRole("button", { name: "刷新", exact: true }).click();
    await expect(results.getByRole("button", { name: "刷新", exact: true })).toBeEnabled();
    await expect(row).toBeVisible();
    await row.getByRole("button", { name: "检测详情" }).click();
    await expect(page.getByRole("dialog").getByRole("heading")).toHaveText(
      "检测详情 · account-21@example.test",
    );
    await page.keyboard.press("Escape");
    await row.getByRole("button", { name: "启用", exact: true }).click();
    const confirm = page.getByRole("dialog").getByRole("button", { name: "启用所选账号" });
    expect(writes).toEqual([]);
    await confirm.click();
    await expect.poll(() => writes).toEqual([{ revision: 2, ids: ["21"] }]);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await results.getByRole("button", { name: "转到下一页" }).scrollIntoViewIfNeeded();
    await page.screenshot({
      path: info.outputPath(`workbench-pagination-${compact ? "compact" : "default"}.png`),
    });
    expect(errors).toEqual([]);
  });
}
