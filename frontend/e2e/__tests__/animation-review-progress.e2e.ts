import { expect, test } from "@playwright/test";
import type { AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

for (const outcome of ["normal", "unknown"] as const) {
  test(`动画在质量标注结束前可预览，标注返回 ${outcome} 后保留原动画`, async ({
    page,
  }, testInfo) => {
    let finished = false;
    const time = new Date().toISOString();
    const result: AnimationResult = {
      account_id: "41",
      account_name: "即时预览账号",
      request_id: "review-request",
      model: "test-model",
      status: "succeeded",
      phase: "reviewing",
      duration_ms: 1500,
      completed_at: time,
      html: '<html><body style="margin:0;background:#eef"><svg viewBox="0 0 640 400"><circle cx="320" cy="200" r="80" fill="#6495ed"/></svg></body></html>',
      source: "<html>original</html>",
      prompt: "绘制动画",
    };
    const task: Task = {
      id: "review-progress",
      skill: "sub2api-model-animation",
      operation: "account-model-animation",
      status: "running",
      progress: 0,
      message: "自动评审中",
      created_at: time,
      updated_at: time,
      result: { account_ids: ["41"], animations: [result] },
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const current: Task = finished
        ? {
            ...task,
            status: "succeeded",
            progress: 100,
            result: {
              ...task.result,
              animations: [
                {
                  ...result,
                  phase: "completed",
                  quality: {
                    status: outcome,
                    score: null,
                    benchmark: "test",
                    reason: outcome === "unknown" ? "review_failed" : undefined,
                  },
                },
              ],
            },
          }
        : task;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "测试用户" },
        "/api/accounts": [{ ...account, id: "41", name: result.account_name, platform: "openai" }],
        "/api/model-checks/animations": [{ ...current, result: {} }],
        "/api/tasks/review-progress": current,
        "/api/model-checks/animation-schedules": [],
        "/api/model-checks/account-statuses": [],
        "/api/dictionaries": { items: [] },
      };
      if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto("/animation-check");
    const card = page.getByRole("article", { name: "账号 即时预览账号" });
    const preview = card.getByRole("button", { name: "放大查看 即时预览账号 的动画" });
    await expect(preview).toBeEnabled();
    await expect(card.locator("iframe")).toBeVisible();
    await expect(card.getByRole("status", { name: "质量标注中" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
    await expect(card.getByRole("button", { name: "重测 即时预览账号" })).toBeDisabled();
    await preview.scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath("animation-reviewing.png") });
    await preview.click();
    const dialog = page.getByRole("dialog", { name: "动画预览", exact: true });
    await expect(dialog.locator("iframe")).toBeVisible();
    await dialog.getByRole("tab", { name: "代码", exact: true }).click();
    await expect(dialog.getByText("<html>original</html>", { exact: true })).toBeVisible();
    await dialog.getByRole("tab", { name: "质量标注", exact: true }).click();
    await expect(dialog.getByRole("status", { name: "质量标注中" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(preview).toBeFocused();
    const frame = await card.locator("iframe").elementHandle();
    finished = true;
    await expect(
      card.getByLabel(`自动质量：${outcome === "normal" ? "正常" : "评审失败"}`, { exact: true }),
    ).toBeVisible();
    await expect(card.getByRole("status", { name: "质量标注中" })).toHaveCount(0);
    expect(await frame?.evaluate((node) => node.isConnected)).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });
}
