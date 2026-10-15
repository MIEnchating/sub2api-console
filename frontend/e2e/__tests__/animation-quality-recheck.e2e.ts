import { expect, test } from "@playwright/test";
import type { AnimationQualityReview, AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("卡片直接复核后可恢复进度，关闭弹窗仍更新标签且旧任务不覆盖新结果", async ({
  page,
}, testInfo) => {
  let complete = false;
  let submissions = 0;
  let review: AnimationQualityReview = {
    sample_id: "sample",
    source_sha256: "source",
    prompt_sha256: "prompt",
    benchmark: "test",
    recheck_version: 0,
    prediction: { status: "suspicious", score: null, benchmark: "test", explanation: "原评审理由" },
    annotation: {
      label: "normal",
      reason: "人工已确认",
      source_group: "",
      version: 1,
      updated_at: "2026-10-15T00:00:00Z",
      updated_by: "operator",
    },
  };
  const originalPrediction = review.prediction;
  const result: AnimationResult = {
    account_id: "41",
    account_name: "复核测试账号",
    request_id: "request-1",
    model: "test-model",
    status: "succeeded",
    phase: "completed",
    duration_ms: 100,
    completed_at: "2026-10-15T00:00:00Z",
    html: "<html><body>动画</body></html>",
    source: "<html>original</html>",
    prompt: "固定原题目",
  };
  const task: Task = {
    id: "source-task",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "succeeded",
    progress: 100,
    message: "完成",
    created_at: result.completed_at,
    updated_at: result.completed_at,
    result: { account_ids: ["41"], animations: [result] },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/animations/source-task/quality/recheck") {
      submissions++;
      expect(route.request().postDataJSON()).toEqual({
        account_id: "41",
        request_id: "request-1",
        version: 0,
      });
      review = {
        ...review,
        recheck_version: 1,
        recheck: { task_id: "recheck-task", status: "running", message: "正在复核原始动画" },
      };
      await route.fulfill({
        json: { ...task, id: "recheck-task", status: "queued", message: "复核已排队" },
      });
      return;
    }
    if (path === "/api/model-checks/animations/source-task/quality") {
      if (complete)
        review = {
          ...review,
          recheck_version: 2,
          prediction: {
            status: "normal",
            score: null,
            benchmark: "test",
            explanation: "复核确认主体和运动正常",
          },
          recheck: { task_id: "recheck-task", status: "succeeded", message: "质量复核完成" },
        };
      await route.fulfill({ json: review });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试用户" },
      "/api/accounts": [{ ...account, id: "41", name: result.account_name, platform: "openai" }],
      "/api/model-checks/animations": [{ ...task, result: {} }],
      "/api/tasks/source-task": {
        ...task,
        result: { account_ids: ["41"], animations: [{ ...result, quality: originalPrediction }] },
      },
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/account-statuses": [],
      "/api/dictionaries": { items: [] },
    };
    if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  const card = page.getByRole("article", { name: "账号 复核测试账号" });
  await card.getByRole("button", { name: "复核", exact: true }).click();
  await expect(card.getByRole("button", { name: "正在复核" })).toBeDisabled();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await card.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("quality-recheck-card.png"), fullPage: true });
  const open = page.getByRole("button", { name: "放大查看 复核测试账号 的动画" });
  await open.click();
  const dialog = page.getByRole("dialog", { name: "动画预览", exact: true });
  await dialog.getByRole("tab", { name: "质量标注" }).click();
  await expect(dialog.getByRole("button", { name: "正在复核" })).toBeDisabled();
  await page.reload();
  await open.click();
  await dialog.getByRole("tab", { name: "质量标注" }).click();
  await expect(dialog.getByRole("button", { name: "正在复核" })).toBeDisabled();
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  complete = true;
  await expect(card.getByLabel("质量标注：正常")).toBeVisible();
  await expect(card.getByRole("button", { name: "复核", exact: true })).toBeEnabled();
  await open.click();
  await dialog.getByRole("tab", { name: "质量标注" }).click();
  await expect(dialog.getByText("复核确认主体和运动正常")).toBeVisible();
  await expect(dialog.getByText("已标注：正常", { exact: true })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("quality-recheck.png"), fullPage: true });
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("article", { name: "账号 复核测试账号" }).getByLabel("质量标注：正常"),
  ).toBeVisible();
  expect(submissions).toBe(1);
});
