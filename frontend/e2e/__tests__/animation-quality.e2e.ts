import { expect, test } from "@playwright/test";
import type { AnimationQualityReview, AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("批次仍运行时已完成动画可在桌面和手机标注，刷新后按原请求回显", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.clock.setFixedTime(new Date("2026-10-14T08:00:00Z"));
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  let review: AnimationQualityReview = {
    prediction: {
      status: "unknown",
      score: null,
      reason: "model_unavailable",
      benchmark: "pelican-v1",
    },
    sample_id: "sample-1",
    benchmark: "pelican-v1",
    source_sha256: "source-1",
    prompt_sha256: "prompt-1",
  };
  const result: AnimationResult = {
    account_id: "41",
    account_name: "质量标注测试账号",
    request_id: "request-1",
    model: "test-model",
    status: "succeeded",
    phase: "completed",
    duration_ms: 1200,
    completed_at: "2026-10-14T00:01:00Z",
    html: '<html><body style="background:#e0f2fe"><svg viewBox="0 0 640 400"><circle cx="320" cy="200" r="75" fill="#059669"/></svg></body></html>',
    source: "<html>original</html>",
    prompt: "固定测试题目",
    quality: review.prediction,
  };
  const task: Task = {
    id: "quality-task",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "running",
    progress: 50,
    message: "已完成 1/2 个账号",
    created_at: "2026-10-14T00:00:00Z",
    updated_at: result.completed_at,
    result: { account_ids: ["41", "42"], animations: [result], completed: 1, total: 2 },
  };
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/model-checks/animations/quality-task/quality") {
      if (route.request().method() === "PUT") {
        const payload = route.request().postDataJSON();
        expect(payload).toEqual({
          account_id: "41",
          request_id: "request-1",
          label: "suspicious",
          reason: "脚踏与腿部脱节，循环时跳帧",
          source_group: "骑车模板甲",
          version: 0,
        });
        review = {
          ...review,
          annotation: {
            label: payload.label,
            reason: payload.reason,
            source_group: payload.source_group,
            version: 1,
            updated_at: result.completed_at,
            updated_by: "测试用户",
          },
        };
      } else {
        expect(url.searchParams.get("account_id")).toBe("41");
        expect(url.searchParams.get("request_id")).toBe("request-1");
      }
      await route.fulfill({ json: review });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试用户" },
      "/api/accounts": [{ ...account, id: "41", name: result.account_name, platform: "openai" }],
      "/api/model-checks/animations": [{ ...task, result: {} }],
      "/api/tasks/quality-task": task,
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/account-statuses": [],
      "/api/dictionaries": { items: [] },
    };
    if (url.pathname.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (url.pathname in fixtures) await route.fulfill({ json: fixtures[url.pathname] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  const card = page.getByRole("article", { name: "账号 质量标注测试账号" });
  await expect(card.getByLabel("自动质量：无法判定")).toBeVisible();
  await card.getByRole("button", { name: "放大查看 质量标注测试账号 的动画" }).click();
  const preview = page.getByRole("dialog", { name: "动画预览", exact: true });
  await preview.getByRole("tab", { name: "质量标注" }).click();
  await expect(preview.getByText("模型未配置，先积累人工标注样本")).toBeVisible();
  await preview.getByRole("button", { name: "人工纠正" }).click();
  await preview.getByRole("radio", { name: "降智" }).check();
  await preview.getByLabel("判断理由（必填）").fill("脚踏与腿部脱节，循环时跳帧");
  await preview.getByLabel("样本分组（可选）").fill("骑车模板甲");
  const save = preview.getByRole("button", { name: "保存质量标注" });
  await save.scrollIntoViewIfNeeded();
  await expect(save).toBeInViewport();
  await save.click();
  await expect(preview.getByText(/已标注：降智/)).toBeVisible();
  expect(await preview.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("quality-review.png"), fullPage: true });
  await page.reload();
  await card.getByRole("button", { name: "放大查看 质量标注测试账号 的动画" }).click();
  await preview.getByRole("tab", { name: "质量标注" }).click();
  await preview.getByRole("button", { name: "人工纠正" }).click();
  await expect(preview.getByRole("radio", { name: "降智" })).toBeChecked();
  await expect(preview.getByLabel("判断理由（必填）")).toHaveValue("脚踏与腿部脱节，循环时跳帧");
  await expect(preview.getByLabel("自动质量：无法判定")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(preview).not.toBeVisible();
});

test("统一自动标注设置在手机和桌面可搜索账号并独立保存超时", async ({ page }, testInfo) => {
  let saved: unknown;
  let timeout = { timeout_seconds: 120, version: 0 };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/timeouts/review") {
      if (route.request().method() === "PUT")
        timeout = { ...route.request().postDataJSON(), version: timeout.version + 1 };
      await route.fulfill({ json: timeout });
      return;
    }
    if (path === "/api/model-checks/animation-quality/settings") {
      if (route.request().method() === "PUT") {
        saved = route.request().postDataJSON();
        await route.fulfill({ json: { ...route.request().postDataJSON(), version: 1 } });
      } else
        await route.fulfill({
          json: { enabled: false, account_id: "", model: "", ...timeout },
        });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试用户" },
      "/api/accounts": Array.from({ length: 40 }, (_, index) => ({
        ...account,
        id: String(index + 41),
        name: index === 0 ? "统一评审账号" : `长名称评审账号用于检查下拉框边界 ${index}`,
      })),
      "/api/model-checks/timeouts/animation": { timeout_seconds: 120, version: 0 },
      "/api/model-checks/animations/accounts/41/models": {
        models: ["review-model", "review-model-alternative"],
      },
      "/api/model-checks/animations": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/account-statuses": [],
      "/api/dictionaries": { items: [] },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/animation-check");
  await page.getByRole("button", { name: "自动标注设置" }).click();
  const dialog = page.getByRole("dialog", { name: "自动质量标注设置" });
  await dialog.getByRole("checkbox", { name: "生成后自动标注质量" }).check();
  await dialog.getByRole("combobox", { name: "评审账号" }).click();
  await page.getByRole("textbox", { name: "搜索选项" }).fill("统一评审账号");
  await page.getByRole("option", { name: "统一评审账号 · 41" }).click();
  await dialog.getByRole("spinbutton", { name: "评审超时（秒）" }).fill("90");
  await dialog.getByRole("button", { name: "保存评审超时" }).click();
  await expect(dialog.getByRole("button", { name: "保存评审超时" })).toBeDisabled();
  await dialog.getByRole("button", { name: "获取模型" }).click();
  const model = dialog.getByRole("combobox", { name: "评审模型" });
  await expect(model).toBeEnabled();
  await model.click();
  const option = page.getByRole("option", { name: "review-model", exact: true });
  await expect(option).toBeInViewport();
  await page.screenshot({ path: testInfo.outputPath("review-model-dropdown.png") });
  await option.click();
  await expect(model).toContainText("review-model");
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("quality-settings.png"), fullPage: true });
  await dialog.getByRole("button", { name: "保存设置" }).click();
  await expect(dialog).not.toBeVisible();
  expect(saved).toEqual({
    enabled: true,
    account_id: "41",
    model: "review-model",
    timeout_seconds: 90,
    version: 1,
  });
  await page.reload();
  await page.getByRole("button", { name: "自动标注设置" }).click();
  await expect(page.getByRole("spinbutton", { name: "评审超时（秒）" })).toHaveValue("90");
});
