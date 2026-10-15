import { expect, test } from "@playwright/test";
import type { AnimationQualityReview, AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("卡片图标统一且确认后刷新保留，页面统一导出全部账号样本", async ({ page }, testInfo) => {
  let writes = 0;
  const exportCursors: string[] = [];
  let review: AnimationQualityReview = {
    sample_id: "sample",
    source_sha256: "source",
    prompt_sha256: "prompt",
    benchmark: "test",
    prediction_sha256: "review-fingerprint",
    recheck_version: 2,
    prediction: {
      source: "model_review",
      status: "normal",
      explanation: "主体完整，运动连续",
      basis: [
        {
          requirement: "车轮转动",
          evidence: "wheel.style.transform = rotation",
          assessment: "每帧更新车轮旋转角度，支持主要转动要求。",
        },
      ],
      score: null,
      benchmark: "test",
      model_version: "review-model",
    },
  };
  const result: AnimationResult = {
    account_id: "41",
    account_name: "收录测试账号",
    request_id: "request",
    model: "test-model",
    status: "succeeded",
    phase: "completed",
    duration_ms: 100,
    completed_at: "2026-10-15T00:00:00Z",
    html: "<html><body>动画</body></html>",
    source: "<html>original</html>",
    prompt: "固定原题目",
    quality: review.prediction,
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
    if (path === "/api/model-checks/animations/source-task/quality/confirm") {
      writes++;
      expect(route.request().postDataJSON()).toEqual({
        account_id: "41",
        request_id: "request",
        version: 0,
        recheck_version: 2,
        prediction_sha256: "review-fingerprint",
      });
      review = {
        ...review,
        annotation: {
          source: "confirmed_auto",
          label: "normal",
          reason: review.prediction.explanation!,
          source_group: "sha256:source",
          prediction_sha256: review.prediction_sha256,
          confirmed_prediction: review.prediction,
          version: 1,
          updated_at: result.completed_at,
          updated_by: "operator",
        },
      };
      await route.fulfill({ json: review });
      return;
    }
    if (path === "/api/model-checks/animations/source-task/quality") {
      await route.fulfill({ json: review });
      return;
    }
    if (path === "/api/model-checks/animation-quality/samples") {
      expect(review.annotation?.source).toBe("confirmed_auto");
      const params = new URL(route.request().url()).searchParams;
      expect(params.has("account_id")).toBe(false);
      const after = params.get("after") ?? "";
      exportCursors.push(after);
      if (after) {
        await route.fulfill({
          json: {
            items: [
              {
                id: "another-sample",
                account_id: "42",
                html: "另一个账号的原文",
                annotation: { label: "suspicious", reason: "主体缺失", source: "manual" },
              },
            ],
          },
        });
        return;
      }
      await route.fulfill({
        json: {
          next_cursor: "sample",
          items: [
            {
              id: "sample",
              task_id: task.id,
              account_id: result.account_id,
              request_id: result.request_id,
              benchmark: "test",
              html: result.source,
              prompt: result.prompt,
              prediction: review.prediction,
              annotation: review.annotation,
            },
          ],
        },
      });
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
        result: {
          account_ids: ["41"],
          animations: [
            {
              ...result,
              quality_annotation: review.annotation,
              quality_prediction_sha256: review.prediction_sha256,
            },
          ],
        },
      },
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/account-statuses": [],
      "/api/dictionaries": { items: [] },
    };
    if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  const card = page.getByRole("article", { name: "账号 收录测试账号" });
  await expect(card.getByRole("button", { name: /^(降智|不降智|清除标记)$/ })).toHaveCount(0);
  const actions = card.getByRole("group", { name: "账号操作", exact: true });
  for (const label of ["确认收录", "复核", "自动检测设置"]) {
    const button = actions.getByRole("button", { name: label, exact: true });
    await expect(button).toHaveAttribute("data-variant", "ghost");
    const box = (await button.boundingBox())!;
    expect(box.width).toBe(32);
    expect(box.height).toBe(32);
  }
  expect((await card.locator("footer").boundingBox())!.height).toBe(40);
  await actions.getByRole("button", { name: "复核", exact: true }).focus();
  await page.keyboard.press("Shift+Tab");
  await expect(actions.getByRole("button", { name: "确认收录", exact: true })).toBeFocused();
  await expect(page.getByRole("tooltip")).toHaveText("确认收录");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("tooltip")).toHaveCount(0);
  await page.keyboard.press("Tab");
  await expect(actions.getByRole("button", { name: "复核", exact: true })).toBeFocused();
  await expect(page.getByRole("tooltip")).toHaveText("复核原动画质量");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("tooltip")).toHaveCount(0);
  await card.getByRole("button", { name: "确认收录", exact: true }).click();
  await expect(card.getByRole("button", { name: "已收录", exact: true })).toBeDisabled();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await card.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("quality-actions-card.png"), fullPage: true });
  const open = page.getByRole("button", { name: "放大查看 收录测试账号 的动画" });
  const dialog = page.getByRole("dialog", { name: "动画预览", exact: true });
  await open.click();
  await dialog.getByRole("tab", { name: "质量标注" }).click();
  await dialog.getByRole("button", { name: /原文依据/ }).click();
  await expect(dialog.getByRole("region", { name: "自动判断依据" })).toContainText("车轮转动");
  await expect(dialog.getByText("wheel.style.transform = rotation", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("textbox")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "已收录训练样本" })).toBeDisabled();
  await page.reload();
  await expect(card.getByRole("button", { name: "已收录", exact: true })).toBeDisabled();
  await open.click();
  await dialog.getByRole("tab", { name: "质量标注" }).click();
  await expect(dialog.getByRole("button", { name: "已收录训练样本" })).toBeDisabled();
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: /导出|样本管理/ })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  const exportButton = page.getByRole("button", { name: "导出全部样本", exact: true });
  await expect(exportButton).toHaveCount(1);
  await expect(exportButton).toHaveAttribute("data-variant", "outline");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const download = page.waitForEvent("download");
  await exportButton.click();
  const file = await download;
  const stream = await file.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  const exported = JSON.parse(Buffer.concat(chunks).toString());
  expect(exported.items[0]).toMatchObject({
    html: result.source,
    prompt: result.prompt,
    annotation: { source: "confirmed_auto", label: "normal", reason: "主体完整，运动连续" },
  });
  expect(exported.items).toHaveLength(2);
  expect(exported.items[1]).toMatchObject({ account_id: "42", html: "另一个账号的原文" });
  expect(exportCursors).toEqual(["", "sample"]);
  expect(writes).toBe(1);
  await page.screenshot({ path: testInfo.outputPath("quality-confirm.png"), fullPage: true });
});
