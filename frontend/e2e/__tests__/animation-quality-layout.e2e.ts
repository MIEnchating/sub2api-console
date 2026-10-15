import { expect, test } from "@playwright/test";
import type { AnimationQualityReview, AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("长理由和多项证据在内容区滚动，收录操作始终可见且手机端不横向溢出", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const review: AnimationQualityReview = {
    sample_id: "sample-layout",
    source_sha256: "source",
    prompt_sha256: "prompt",
    benchmark: "test",
    prediction_sha256: "fingerprint",
    recheck_version: 0,
    prediction: {
      status: "normal",
      source: "model_review",
      score: null,
      benchmark: "test",
      model_version: "review-model-with-a-long-name-".repeat(5),
      explanation:
        "主体结构与主要运动具备源码支持，允许次要装饰存在轻微瑕疵。\n" +
        "车轮关键帧持续更新旋转角度，腿部位置随踏板轨迹变化。\n".repeat(14) +
        "本次仅依据源码，未运行或观看动画。",
      basis: [
        {
          requirement: "主体结构完整",
          assessment: "主体通过分层 SVG 绘制，主要部件可识别。",
          evidence:
            '<svg data-description="' + "long-source-".repeat(150) + '"><g id="body"/></svg>',
        },
        {
          requirement: "车轮连续旋转",
          assessment: "关键帧与车轮绑定，提供持续旋转。",
          evidence: "@keyframes spin { to { transform: rotate(360deg) } }",
        },
        {
          requirement: "踩踏动作联动",
          assessment: "脚部按踏板轨迹更新坐标。",
          evidence: "foot.x = pedal.x;\nfoot.y = pedal.y;",
        },
      ],
    },
  };
  const result: AnimationResult = {
    account_id: "41",
    account_name: "布局测试账号",
    request_id: "layout-request",
    model: "test-model",
    status: "succeeded",
    phase: "completed",
    duration_ms: 100,
    completed_at: "2026-10-15T00:00:00Z",
    html: "<html><body>动画</body></html>",
    source: "<html>original</html>",
    prompt: "固定题目",
    quality: review.prediction,
  };
  const task: Task = {
    id: "layout-task",
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
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试用户" },
      "/api/accounts": [{ ...account, id: "41", name: result.account_name, platform: "openai" }],
      "/api/model-checks/animations": [{ ...task, result: {} }],
      "/api/tasks/layout-task": task,
      "/api/model-checks/animations/layout-task/quality": review,
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/account-statuses": [],
      "/api/dictionaries": { items: [] },
    };
    if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("button", { name: "放大查看 布局测试账号 的动画" }).click();
  const dialog = page.getByRole("dialog", { name: "动画预览", exact: true });
  await dialog.getByRole("tab", { name: "质量标注" }).click();
  await dialog
    .getByText(`模型自动评审 · ${review.prediction.model_version}`, { exact: true })
    .focus();
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Tab");
  await expect(page.getByRole("tooltip")).toHaveText(review.prediction.model_version!);
  await page.keyboard.press("Escape");
  const content = dialog.getByRole("region", { name: "标注内容", exact: true });
  const confirm = dialog.getByRole("button", { name: "确认结果并收录样本" });
  await expect(confirm).toBeEnabled();
  await expect(confirm).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "重新评审", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(await content.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("quality-layout.png") });

  const evidence = dialog.getByRole("button", { name: /原文依据/ });
  await expect(evidence).toHaveAttribute("aria-expanded", "false");
  await evidence.scrollIntoViewIfNeeded();
  await evidence.focus();
  await page.keyboard.press("Enter");
  await expect(evidence).toHaveAttribute("aria-expanded", "true");
  const source = dialog.getByLabel("主体结构完整的原文证据");
  await source.scrollIntoViewIfNeeded();
  await expect(source).toBeInViewport();
  await expect(confirm).toBeInViewport({ ratio: 1 });
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  expect(await content.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("quality-evidence.png") });

  await dialog.getByRole("button", { name: "人工纠正" }).click();
  const reason = dialog.getByLabel("判断理由（必填）");
  await reason.fill("人工复核补充判断");
  await expect(reason).toHaveValue("人工复核补充判断");
  await expect(confirm).toBeInViewport({ ratio: 1 });
  await dialog.getByRole("button", { name: "保存质量标注" }).scrollIntoViewIfNeeded();
  await expect(dialog.getByRole("button", { name: "保存质量标注" })).toBeInViewport({ ratio: 1 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("button", { name: "放大查看 布局测试账号 的动画" })).toBeFocused();
});
