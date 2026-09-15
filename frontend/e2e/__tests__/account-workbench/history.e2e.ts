import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("处理记录长结果筛选和详情在桌面手机保持可见", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const id = "account-job-" + "long-identifier".repeat(12);
  await page.route("**/api/account-workbench/history", (route) =>
    route.fulfill({
      json: [
        {
          id,
          skill: "account-workbench",
          operation: "account-workbench-import",
          status: "failed",
          progress: 100,
          message: "长账号结果_" + "LongAccountName".repeat(20),
          created_at: "2026-09-14T00:00:00Z",
          updated_at: "2026-09-14T00:00:00Z",
          result: { email: "owner@example.com" },
        },
      ],
    }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "处理记录", exact: true }).click();
  await page.getByRole("textbox", { name: "搜索处理记录" }).fill("owner@example.com");
  await page.getByRole("button", { name: `查看任务 ${id}` }).click();
  const dialog = page.getByRole("dialog", { name: "处理详情" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "关闭详情" }).scrollIntoViewIfNeeded();
  await expect(dialog.getByRole("button", { name: "关闭详情" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("history-selection.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "关闭详情" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("checkbox")).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "搜索处理记录" })).toHaveValue(
    "owner@example.com",
  );
});
