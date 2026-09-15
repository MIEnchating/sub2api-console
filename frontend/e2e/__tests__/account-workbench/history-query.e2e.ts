import { expect, test } from "@playwright/test";
import type { Task, WorkbenchHistoryQuery } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

const prior: Task = {
  id: "earlier-task",
  skill: "account-workbench",
  operation: "account-workbench-import",
  status: "running",
  message: "旧任务_" + "long-account-description".repeat(35),
  progress: 20,
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
  result: {},
};

test("全部历史查询在桌面手机可批量输入邮箱并保留分页操作", async ({ page }) => {
  await installWorkbenchFixture(page);
  const queries: WorkbenchHistoryQuery[] = [];
  await page.route("**/api/account-workbench/history/query", (route) => {
    const input = route.request().postDataJSON() as WorkbenchHistoryQuery;
    queries.push(input);
    return route.fulfill({
      json: {
        items: [{ ...prior, status: "succeeded", id: `prior-${input.offset}` }],
        total: 125,
        offset: input.offset,
        limit: 50,
      },
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "处理记录", exact: true }).click();
  await page.getByRole("button", { name: "查询全部历史" }).click();
  const dialog = page.getByRole("dialog", { name: "查询全部历史" });
  await expect(dialog.getByRole("button", { name: "下一页" })).toBeInViewport();
  await dialog
    .getByRole("textbox", { name: "批量查询邮箱" })
    .fill("ALICE@example.com\nbob@example.com");
  await dialog.getByRole("button", { name: "查询记录" }).click();
  await expect.poll(() => queries.at(-1)?.emails).toEqual(["alice@example.com", "bob@example.com"]);
  await dialog.getByRole("button", { name: "下一页" }).click();
  await expect(dialog).toContainText("共 125 条 · 第 2 页");
  await dialog.getByRole("button", { name: "下一页" }).click();
  await expect(dialog).toContainText("共 125 条 · 第 3 页");
  await expect(dialog.getByRole("button", { name: "下一页" })).toBeDisabled();
  await expect(dialog.getByRole("button", { name: "上一页" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("history-all.png"),
    animations: "disabled",
  });
});

test("全部任务取消在桌面手机展示旧任务范围且确认按钮固定可用", async ({ page }) => {
  await installWorkbenchFixture(page);
  await page.route("**/api/account-workbench/history", (route) => route.fulfill({ json: [] }));
  const tasks = Array.from({ length: 110 }, (_, index) => ({ ...prior, id: `earlier-${index}` }));
  await page.route("**/api/account-workbench/history/active", (route) =>
    route.fulfill({ json: tasks }),
  );
  const cancellations: Array<{ items: { id: string }[]; confirmed: boolean }> = [];
  await page.route("**/api/account-workbench/history/cancel", (route) => {
    const body = route.request().postDataJSON() as (typeof cancellations)[number];
    cancellations.push(body);
    return route.fulfill({
      json: {
        items: body.items.map((item) => ({ id: item.id, cancelled: true, message: "已请求取消" })),
      },
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "处理记录", exact: true }).click();
  await page.getByRole("button", { name: "取消全部工作台任务" }).click();
  const dialog = page.getByRole("dialog", { name: "确认取消全部工作台任务" });
  await expect(dialog).toContainText("共 110 个活动任务");
  await expect(dialog.getByRole("button", { name: "确认取消这些任务" })).toBeInViewport();
  expect(cancellations).toHaveLength(0);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("history-cancel-all.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "确认取消这些任务" }).click();
  await expect(dialog).not.toBeVisible();
  expect(cancellations[0].confirmed).toBe(true);
  expect(cancellations[0].items).toHaveLength(110);
  expect(cancellations[0].items[109].id).toBe("earlier-109");
});
