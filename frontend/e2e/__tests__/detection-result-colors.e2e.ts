import { expect, test } from "@playwright/test";
import type { AnimationResult, DetectionTask, Task } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("检测详情通过、降智、无法判定和等待使用不同状态色且手机无溢出", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const plan: DetectionTask = {
    id: "colors",
    version: 1,
    name: "自动前置检测",
    group_ids: ["7"],
    model: "test-model",
    animation: false,
    precheck: true,
    terminal: false,
    terminal_rounds: 1,
    automatic: false,
    schedule_type: "interval",
    interval_minutes: 60,
    timeout_seconds: 120,
    last_task_id: "color-run",
  };
  const results: AnimationResult[] = (
    ["passed", "not_passed", "inconclusive", "error"] as const
  ).map((verdict, index) => ({
    account_id: String(index + 1),
    account_name: `测试账号 ${index + 1}`,
    model: "test-model",
    mode: "precheck",
    status: verdict === "error" ? "failed" : "succeeded",
    request_id: `request-${index}`,
    duration_ms: 100,
    completed_at: "2026-09-24T00:00:00Z",
    precheck: {
      verdict,
      profile_version: "candy-v1",
      questions: [
        {
          id: "candy",
          verdict,
          request_id: `question-${index}`,
          answer: verdict === "passed" ? "21" : "18",
        },
      ],
    },
  }));
  const run: Task = {
    id: "color-run",
    skill: "sub2api-model-animation",
    operation: "managed-model-detection",
    status: "running",
    progress: 80,
    message: "账号 5：前置检测",
    created_at: "2026-09-24T00:00:00Z",
    updated_at: "2026-09-24T00:00:10Z",
    result: {
      configuration: plan,
      account_ids: ["1", "2", "3", "4", "5"],
      animations: results,
      total: 5,
      completed: 4,
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "test" },
      "/api/accounts": [],
      "/api/groups": [{ id: "7", name: "测试分组" }],
      "/api/model-checks/detection-tasks": [plan],
      "/api/model-checks/animations": [],
      "/api/model-checks/animation-schedules": [],
      "/api/tasks/color-run": run,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/animation-check");
  await page.getByRole("tab", { name: "任务管理", exact: true }).click();
  await page.getByRole("button", { name: "运行详情", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测任务运行详情", exact: true });
  const colors: string[] = [];
  const backgrounds: string[] = [];
  for (const label of ["通过", "降智", "无法判定", "等待检测结果"]) {
    const status = dialog.getByRole("group", { name: `前置检测 · ${label}`, exact: true });
    await expect(status).toBeVisible();
    const style = await status.evaluate((element) => ({
      color: getComputedStyle(element).color,
      background: getComputedStyle(element).backgroundColor,
    }));
    colors.push(style.color);
    backgrounds.push(style.background);
    expect(await status.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
  }
  expect(new Set(colors).size).toBe(4);
  expect(new Set(backgrounds).size).toBe(4);
  expect(
    await dialog
      .getByRole("group", { name: "前置检测 · 请求失败", exact: true })
      .evaluate((element) => getComputedStyle(element).color),
  ).toBe(colors[1]);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const trigger = dialog.getByRole("button", { name: "查看前置检测详情" }).first();
  await trigger.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "前置检测详情", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
  await page.screenshot({ path: test.info().outputPath("detection-result-colors.png") });
  run.status = "failed";
  run.message = "任务执行失败：context deadline exceeded";
  run.result.error = "context deadline exceeded";
  await dialog.getByRole("button", { name: "关闭", exact: true }).click();
  await page.getByRole("button", { name: "运行详情", exact: true }).click();
  const pending = dialog.getByRole("article", { name: "检测账号 账号 5", exact: true });
  await expect(
    pending.getByRole("group", { name: "前置检测 · 任务超时，未取得结果", exact: true }),
  ).toBeVisible();
  await expect(pending).toContainText("可能尚未执行或执行中断");
  await expect(dialog.getByText(/context deadline exceeded/)).toHaveCount(0);
  await expect(
    dialog.getByRole("group", { name: "前置检测 · 请求失败", exact: true }),
  ).toBeVisible();
  await expect(dialog.getByRole("heading", { name: "测试分组", exact: true })).toHaveCount(0);
  await expect(dialog.getByRole("tab", { name: /测试分组/ })).toContainText("5");
  expect(await pending.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("detection-timeout-reason.png") });
});
