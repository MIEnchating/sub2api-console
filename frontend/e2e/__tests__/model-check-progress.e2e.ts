import { expect, test } from "@playwright/test";
import type { Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("检测中可查看逐项原文，实时更新保留展开位置且取消入口保持可见", async ({ page }, testInfo) => {
  const first = {
    id: "arithmetic",
    label: "多步计算",
    round: 1,
    request_id: "probe-1",
    status: "passed",
    prompt: "计算 7 × 3",
    expected: "21",
    answer: "**21**",
    protocol: "openai_responses",
    http_status: 200,
    duration_ms: 250,
  };
  const second = {
    id: "constraints",
    label: "约束推理",
    round: 1,
    request_id: "probe-2",
    status: "running",
    prompt: "约束推理题目",
    expected: "42",
    started_at: new Date().toISOString(),
  };
  const plan = [
    first,
    second,
    ...["代码状态推演", "路径约束推理", "上下文信息提取", "指令与结构化输出", "重复请求一致性"].map(
      (label, index) => ({ id: `pending-${index}`, label, round: 1 }),
    ),
    ...Array.from({ length: 10 }, (_, index) => ({
      id: `fingerprint-${index}`,
      label: `指纹采样 ${index + 1}`,
      round: 1,
      category: "fingerprint",
    })),
  ];
  const active = {
    account_id: "41",
    account_name: "检测账号",
    claimed_model: "test-model",
    probes: [first],
    current_probe: second,
    probe_plan: plan,
    completed_probes: 1,
    planned_probes: 17,
    timeout_seconds: 45,
  };
  let task: Task = {
    id: "live-progress-task",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "running",
    progress: 17,
    message: "第 1 轮约束推理等待回复",
    created_at: "2026-10-15T00:00:00Z",
    updated_at: "2026-10-15T00:00:01Z",
    result: {
      phase: "testing",
      completed: 0,
      total: 2,
      active: {
        "0": active,
        "1": {
          ...active,
          account_id: "42",
          account_name: "第二个检测账号",
          claimed_model: "long-model-".repeat(12),
        },
      },
      tests: [],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/events")) {
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: `retry: 100\ndata: ${JSON.stringify(task)}\n\n`,
      });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/accounts": [{ ...account, name: "检测账号", platform: "openai" }],
      "/api/accounts/41/models": { models: ["test-model"] },
      "/api/model-checks": task,
      "/api/tasks/live-progress-task": task,
    };
    await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/model-check");
  await page.getByRole("button", { name: "全选账号" }).click();
  await page.getByRole("checkbox", { name: "选择模型 test-model" }).click();
  await page.getByRole("button", { name: /开始检测/ }).click();
  const dialog = page.getByRole("dialog", { name: "正在检测模型", exact: true });
  const process = dialog.getByRole("region", { name: "逐项检测过程" });
  const combinations = dialog.getByRole("navigation", { name: "检测组合" });
  await expect(combinations.getByRole("button", { name: /^检测账号/ })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await combinations.getByRole("button", { name: /第二个检测账号/ }).click();
  await expect(process.getByRole("heading", { name: "第二个检测账号" })).toBeVisible();
  await combinations.getByRole("button", { name: /^检测账号/ }).click();
  await expect(process.getByRole("heading", { name: "能力检测" })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("live-layout.png") });
  await expect(dialog.getByText("**21**", { exact: true })).toBeVisible();
  await expect(dialog.getByText("等待回复", { exact: true })).toBeVisible();
  const details = process
    .locator("details")
    .filter({ has: page.getByText("多步计算", { exact: true }) })
    .last();
  await details.locator("summary").focus();
  await page.keyboard.press("Enter");
  await expect(details).toHaveAttribute("open", "");
  await expect(details.getByText("计算 7 × 3", { exact: true })).toBeVisible();
  await expect(details.getByText(/HTTP 200/)).toBeVisible();
  task = {
    ...task,
    progress: 30,
    updated_at: "2026-10-15T00:00:02Z",
    message: "约束推理请求失败",
    result: {
      ...task.result,
      active: {
        "0": {
          ...active,
          completed_probes: 2,
          current_probe: null,
          probes: [
            first,
            { ...second, status: "error", error: "请求超时，请检查接口", duration_ms: 45000 },
          ],
        },
      },
    },
  };
  await expect(process.getByText("请求超时，请检查接口", { exact: true })).toBeVisible();
  await expect(details).toHaveAttribute("open", "");
  await expect(dialog.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "30");
  await process.getByText("指纹采样 10", { exact: true }).scrollIntoViewIfNeeded();
  await expect(dialog.getByRole("button", { name: "取消任务" })).toBeInViewport({ ratio: 1 });
  await expect.poll(() => process.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  expect(
    await dialog.evaluate((element) => element.scrollWidth - element.clientWidth),
  ).toBeLessThanOrEqual(1);
});
