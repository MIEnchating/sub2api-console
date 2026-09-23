import { expect, test } from "@playwright/test";
import type { DetectionTask } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("分组任务可保存多时间与多轮、直接启动并逐轮查看结果", async ({ page }) => {
  let plans: DetectionTask[] = [];
  const runs: unknown[] = [];
  const run = {
    id: "run-1",
    skill: "sub2api-model-animation",
    operation: "managed-model-detection",
    status: "succeeded",
    progress: 100,
    message: "检测任务完成",
    created_at: "2026-09-23T00:00:00Z",
    updated_at: "2026-09-23T00:00:30Z",
    result: {
      completed: 2,
      total: 2,
      checks: [
        {
          account_id: "41",
          account_name: "测试账号",
          model: "test-model",
          request_id: "request",
          verdict: "suspected",
          duration_ms: 30,
          completed_at: "2026-09-23T00:00:30Z",
          round_results: [
            {
              round: 1,
              request_id: "round-1",
              verdict: "normal",
              response: "第一轮正常回答",
              duration_ms: 10,
              completed_at: "2026-09-23T00:00:10Z",
            },
            {
              round: 2,
              request_id: "round-2",
              verdict: "suspected",
              response: "第二轮未延续工具上下文",
              duration_ms: 10,
              completed_at: "2026-09-23T00:00:20Z",
            },
            {
              round: 3,
              request_id: "round-3",
              verdict: "error",
              error: "第三轮请求失败",
              duration_ms: 10,
              completed_at: "2026-09-23T00:00:30Z",
            },
          ],
        },
      ],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/detection-tasks") {
      if (route.request().method() === "PUT")
        plans = [{ ...route.request().postDataJSON(), id: "plan-1", version: 1 }];
      await route.fulfill({ json: plans });
      return;
    }
    if (path === "/api/model-checks/detection-tasks/plan-1/run") {
      runs.push(route.request().postDataJSON());
      plans = [{ ...plans[0], last_task_id: "run-1" }];
      await route.fulfill({ json: run });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "test" },
      "/api/accounts": [],
      "/api/groups": [
        { id: "7", name: "主分组" },
        { id: "8", name: "备用分组" },
      ],
      "/api/model-checks/animations": [],
      "/api/model-checks/animation-schedules": [],
      "/api/tasks/run-1": run,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/animation-check");
  await page.getByRole("tab", { name: "任务管理", exact: true }).click();
  await page.getByRole("button", { name: "新增任务" }).click();
  const editor = page.getByRole("dialog", { name: "新增检测任务" });
  await editor.getByRole("textbox", { name: "任务名称" }).fill("每日分组检测");
  await editor.getByRole("combobox", { name: "检测分组" }).click();
  await page.getByRole("option", { name: "主分组（ID 7）" }).click();
  await page.getByRole("option", { name: "备用分组（ID 8）" }).click();
  await page.keyboard.press("Escape");
  await editor.getByRole("textbox", { name: "检测模型" }).fill("test-model");
  await editor.getByRole("checkbox", { name: "前置检测", exact: true }).check();
  await editor.getByRole("checkbox", { name: "终端检测", exact: true }).check();
  await editor.getByRole("spinbutton", { name: "终端检测轮数" }).fill("3");
  await editor.getByRole("checkbox", { name: "自动检测", exact: true }).check();
  await editor.getByRole("radio", { name: "每天定时" }).check();
  await editor.getByLabel("每天检测时间（北京时间）", { exact: true }).fill("09:00");
  await editor.getByRole("button", { name: "添加检测时间" }).click();
  await editor.getByLabel("每天检测时间 2（北京时间）").fill("20:00");
  expect(await editor.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  await editor.getByRole("button", { name: "保存任务" }).click();
  const confirmation = page.getByRole("dialog", { name: "确认自动检测任务" });
  await expect(confirmation).toContainText("终端检测 3 轮");
  await expect(confirmation).toContainText("09:00、20:00");
  await confirmation.getByRole("button", { name: "确认保存并开启" }).click();
  await expect(editor).toBeHidden();
  expect(plans[0]).toMatchObject({
    group_ids: ["7", "8"],
    precheck: true,
    terminal: true,
    terminal_rounds: 3,
    automatic: true,
    daily_times: ["09:00", "20:00"],
  });
  await page.getByRole("button", { name: "立即执行" }).click();
  await expect.poll(() => runs).toEqual([{ version: 1 }]);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "运行详情" }).click();
  const details = page.getByRole("dialog", { name: "检测任务运行详情" });
  await expect(details.getByRole("heading", { name: "逐轮检测结果（3 轮）" })).toBeVisible();
  const second = details.locator("summary").filter({ hasText: "第 2 轮" });
  await second.focus();
  await page.keyboard.press("Enter");
  await expect(details.getByText("第二轮未延续工具上下文")).toBeVisible();
  await details.locator("summary").filter({ hasText: "第 3 轮" }).click();
  await expect(details.getByText("第三轮请求失败")).toBeVisible();
  expect(await details.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("detection-task-rounds.png") });
});
