import { expect, test } from "@playwright/test";
import type { AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("前置检测结果支持筛选后生成动画及保存定时内容，窄屏控件不溢出", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const accounts = [1, 2].map((id) => ({
    ...account,
    id: String(id),
    name: id === 1 ? "前置通过账号" : "前置不通过账号",
    platform: "openai",
  }));
  const animations: AnimationResult[] = accounts.map((item, i) => ({
    account_id: item.id,
    account_name: item.name,
    model: "gpt-6-astra",
    mode: "precheck",
    status: "succeeded",
    request_id: `precheck-${item.id}`,
    completed_at: "2026-09-15T00:01:00Z",
    duration_ms: 100,
    precheck: {
      verdict: i === 0 ? "passed" : "not_passed",
      profile_version: "astra-v1",
      questions: [
        {
          id: "candy",
          verdict: i === 0 ? "passed" : "not_passed",
          answer: i === 0 ? "21" : "22",
          request_id: `candy-${item.id}`,
        },
        {
          id: "knowledge-cutoff",
          verdict: "passed",
          answer: "我无法提供知识截止日期。".repeat(12),
          request_id: `cutoff-${item.id}`,
        },
      ],
    },
  }));
  const task: Task = {
    id: "precheck-e2e",
    skill: "sub2api-model-animation",
    operation: "account-model-precheck",
    status: "succeeded",
    progress: 100,
    message: "前置检测完成",
    result: { animations, account_ids: ["1", "2"] },
    created_at: "2026-09-15T00:00:00Z",
    updated_at: "2026-09-15T00:01:00Z",
  };
  let created = false;
  const writes: unknown[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/animations" && route.request().method() === "POST") {
      const body = route.request().postDataJSON();
      expect(body).toEqual({
        mode: "precheck",
        targets: [
          { account_id: "1", model: "gpt-6-astra" },
          { account_id: "2", model: "gpt-6-astra" },
        ],
        precheck_questions: ["candy", "knowledge-cutoff"],
        timeout_seconds: 120,
      });
      created = true;
      await route.fulfill({ json: task });
      return;
    }
    if (path === "/api/model-checks/animation-schedules/1" && route.request().method() === "PUT") {
      writes.push(route.request().postDataJSON());
      await route.fulfill({ json: [] });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "前置检测测试" },
      "/api/accounts": accounts,
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": created ? [{ ...task, result: {} }] : [],
      "/api/tasks/precheck-e2e": task,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/model-check");
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  const panel = page.getByRole("tabpanel", { name: "动画检测", exact: true });
  await panel.getByRole("combobox", { name: "检测模型" }).fill("gpt-6-astra");
  await panel.getByRole("button", { name: "选择前置检测题目" }).click();
  const questions = page.getByRole("dialog", { name: "前置检测题目", exact: true });
  await questions.getByRole("checkbox", { name: "知识截止日期", exact: true }).uncheck();
  await expect(questions.getByRole("checkbox", { name: "全选", exact: true })).toHaveAttribute(
    "aria-checked",
    "mixed",
  );
  await questions.getByRole("checkbox", { name: "全选", exact: true }).check();
  await page.screenshot({ path: testInfo.outputPath("precheck-questions.png") });
  await page.keyboard.press("Escape");
  for (const item of accounts)
    await panel
      .getByRole("article", { name: `账号 ${item.name}`, exact: true })
      .getByRole("checkbox")
      .check();
  await panel.getByRole("button", { name: "前置检测（2）" }).click();
  await page
    .getByRole("dialog", { name: "确认前置检测范围" })
    .getByRole("button", { name: "确认并开始检测" })
    .click();
  const passed = panel.getByRole("article", { name: "账号 前置通过账号", exact: true });
  const rejected = panel.getByRole("article", { name: "账号 前置不通过账号", exact: true });
  await expect(
    passed.getByRole("listitem").filter({ hasText: "糖果题" }).getByText("通过", { exact: true }),
  ).toBeVisible();
  await expect(
    rejected
      .getByRole("listitem")
      .filter({ hasText: "糖果题" })
      .getByText("不通过", { exact: true }),
  ).toBeVisible();
  await panel.getByRole("button", { name: "选择不通过（1）" }).click();
  await expect(rejected.getByRole("checkbox")).toBeChecked();
  await expect(passed.getByRole("checkbox")).not.toBeChecked();
  await panel.getByRole("button", { name: "选择通过（1）" }).click();
  await expect(passed.getByRole("checkbox")).toBeChecked();
  await expect(rejected.getByRole("checkbox")).not.toBeChecked();
  await passed.scrollIntoViewIfNeeded();
  expect((await passed.boundingBox())!.height).toBeLessThanOrEqual(450);
  expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("precheck-results.png") });
  await panel.getByRole("group", { name: "前置检测操作", exact: true }).scrollIntoViewIfNeeded();
  for (const name of ["前置检测（1）", "选择通过（1）", "选择不通过（1）"]) {
    await expect(panel.getByRole("button", { name })).toBeInViewport({ ratio: 1 });
  }
  await panel.getByRole("button", { name: "开始检测（1 个账号）" }).click();
  const next = page.getByRole("dialog", { name: "确认动画检测范围" });
  await expect(next).toContainText("前置通过账号（ID 1）");
  await expect(next).not.toContainText("前置不通过账号（ID 2）");
  await next.getByRole("button", { name: "取消", exact: true }).click();
  await passed.getByRole("button", { name: "自动检测设置" }).click();
  const schedule = page.getByRole("dialog", { name: "自动检测设置 · 前置通过账号", exact: true });
  await schedule.getByRole("checkbox", { name: "前置检测", exact: true }).check();
  await schedule.getByRole("checkbox", { name: "动画检测", exact: true }).uncheck();
  await schedule.getByRole("checkbox", { name: "开启自动检测" }).check();
  await schedule.getByRole("button", { name: "选择前置检测题目" }).click();
  await page
    .getByRole("dialog", { name: "前置检测题目", exact: true })
    .getByRole("checkbox", { name: "糖果题", exact: true })
    .uncheck();
  await page.keyboard.press("Escape");
  await page.screenshot({ path: testInfo.outputPath("precheck-schedule.png") });
  await schedule.getByRole("button", { name: "保存设置", exact: true }).click();
  const confirm = page.getByRole("dialog", { name: "确认开启自动检测", exact: true });
  await expect(confirm).toContainText("知识截止日期题");
  await expect(confirm).not.toContainText("糖果题");
  await confirm.getByRole("button", { name: "确认保存并开启", exact: true }).click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0]).toMatchObject({
    account_id: "1",
    mode: "precheck",
    precheck_questions: ["knowledge-cutoff"],
    enabled: true,
    model: "gpt-6-astra",
  });
});
