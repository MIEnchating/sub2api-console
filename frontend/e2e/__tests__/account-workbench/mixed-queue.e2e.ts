import { expect, test } from "@playwright/test";
import type { WorkbenchRunView } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("混合恢复保留长标识布局、刷新后接回原任务并经确认结束", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const resumes: unknown[] = [];
  const deletes: string[] = [];
  const expires = new Date(Date.now() + 600000).toISOString();
  const run: WorkbenchRunView = {
    id: "mixed-reconnected",
    task_id: "mixed-reconnected",
    status: "ready",
    message: "混合账号结果已恢复",
    expires_at: expires,
    export_only: true,
    available: 1,
    recovery_enabled: true,
    recovery_id: "mixed-saved",
    errors: [],
    items: [
      {
        index: 0,
        kind: "codex_json",
        name: "已保存账号_" + "workspace-identity".repeat(16),
        has_password: false,
        has_totp: false,
        has_proxy: false,
        status: "succeeded",
        message: "凭据已私有保存",
      },
    ],
  };
  await page.route("**/api/account-workbench/queue-recoveries", (route) =>
    route.fulfill({
      json: [
        {
          id: "mixed-saved",
          kind: "mixed",
          task_id: "original-task-" + "stable-id".repeat(25),
          status: "completed",
          revision: 9,
          expires_at: expires,
          can_resume: true,
          total: 1,
          completed: 1,
          pending: 0,
          uncertain: 0,
        },
      ],
    }),
  );
  await page.route("**/api/account-workbench/queue-recoveries/mixed-saved/mixed", async (route) => {
    resumes.push(route.request().postDataJSON() as unknown);
    await route.fulfill({ json: run });
  });
  await page.route("**/api/account-workbench/runs/mixed-reconnected", async (route) => {
    if (route.request().method() === "DELETE") deletes.push(route.request().url());
    await route.fulfill({
      json: route.request().method() === "DELETE" ? { cancelled: true } : run,
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "混合运行", exact: true }).click();
  await page.getByRole("button", { name: "恢复已保存批次" }).click();
  const dialog = page.getByRole("dialog", { name: "恢复已保存批次", exact: true });
  await expect(dialog.getByRole("button", { name: "恢复本批" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("mixed-queue-list.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "恢复本批" }).click();
  expect(resumes).toHaveLength(0);
  await page.getByRole("button", { name: "确认继续本批" }).click();
  const progress = page.getByRole("region", { name: "混合运行进度" });
  await expect(progress).toContainText("mixed-reconnected");
  await expect(page.getByRole("button", { name: "预览私有转换结果" })).toBeEnabled();
  expect((await progress.boundingBox())?.x).toBeGreaterThanOrEqual(0);
  expect(resumes).toEqual([{ revision: 9, confirmed: true }]);
  await page.screenshot({
    path: test.info().outputPath("mixed-queue-restored.png"),
    animations: "disabled",
  });
  await page.reload();
  await page.getByRole("tab", { name: "混合运行", exact: true }).click();
  await page.getByRole("button", { name: "恢复已保存批次" }).click();
  await page.getByRole("button", { name: "恢复本批" }).click();
  await page.getByRole("button", { name: "确认继续本批" }).click();
  await expect(progress).toContainText("mixed-reconnected");
  expect(deletes).toHaveLength(0);
  expect(resumes).toHaveLength(2);
  await page.getByRole("button", { name: "结束混合运行" }).click();
  expect(deletes).toHaveLength(0);
  await page.getByRole("button", { name: "结束并清除混合结果" }).click();
  await expect(progress).toHaveCount(0);
  expect(deletes).toHaveLength(1);
});
