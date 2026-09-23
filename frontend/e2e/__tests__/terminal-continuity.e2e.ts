import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";
import type { Task, TerminalContinuityRequest } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

const accounts = [
  { ...account, id: "41", name: "终端正常账号", platform: "openai" },
  { ...account, id: "42", name: "终端异常账号", platform: "openai" },
];

async function openTerminalContinuity(page: Page): Promise<TerminalContinuityRequest[]> {
  const requests: TerminalContinuityRequest[] = [];
  const completed: Task = {
    id: "terminal-e2e",
    skill: "sub2api-terminal-continuity",
    operation: "account-terminal-continuity",
    status: "succeeded",
    progress: 100,
    message: "终端续接检测完成",
    created_at: "2026-09-22T00:00:00Z",
    updated_at: "2026-09-22T00:01:00Z",
    result: {
      account_ids: ["41", "42"],
      checks: [
        {
          account_id: "41",
          account_name: "终端正常账号",
          model: "gpt-6-astra",
          request_id: "terminal-normal",
          verdict: "normal",
          response: '{"tool":"exec_command","command":"git status --short"}',
          duration_ms: 900,
          completed_at: "2026-09-22T00:01:00Z",
        },
        {
          account_id: "42",
          account_name: "终端异常账号",
          model: "gpt-6-astra",
          request_id: "terminal-suspected",
          verdict: "suspected",
          response: "当前会话没有可用的终端、文件系统或 GitHub 查询工具入口。",
          duration_ms: 1100,
          completed_at: "2026-09-22T00:01:00Z",
        },
      ],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/terminal-continuity" && route.request().method() === "POST") {
      requests.push(route.request().postDataJSON() as TerminalContinuityRequest);
      await route.fulfill({ json: completed });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "终端检测测试" },
      "/api/accounts": accounts,
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [],
      "/api/model-checks/terminal-continuity": requests.length ? [completed] : [],
      "/api/tasks/terminal-e2e": completed,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("tab", { name: "终端续接检测", exact: true }).click();
  await expect(page.getByRole("tabpanel", { name: "终端续接检测", exact: true })).toBeVisible();
  return requests;
}

test("终端续接检测独立提交并筛出无依据否认工具的账号", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const requests = await openTerminalContinuity(page);
  const panel = page.getByRole("tabpanel", { name: "终端续接检测", exact: true });
  await panel.getByRole("button", { name: "全选账号", exact: true }).click();
  await panel.getByRole("combobox", { name: "检测模型" }).fill("gpt-6-astra");
  await page.keyboard.press("Escape");
  await panel.getByRole("spinbutton", { name: "终端检测轮数" }).fill("3");
  await panel.getByRole("button", { name: "开始检测（2 个账号）" }).click();
  const confirm = page.getByRole("dialog", { name: "确认终端续接检测范围" });
  await expect(confirm).toContainText("不修改健康分");
  await confirm.getByRole("button", { name: "确认并开始检测" }).click();
  await expect
    .poll(() => requests)
    .toEqual([
      {
        targets: [
          { account_id: "41", model: "gpt-6-astra" },
          { account_id: "42", model: "gpt-6-astra" },
        ],
        timeout_seconds: 120,
        rounds: 3,
      },
    ]);
  await expect(panel.getByText("正常", { exact: true })).toBeVisible();
  await expect(panel.getByText("疑似无终端权限", { exact: true })).toBeVisible();
  await panel.getByRole("button", { name: "选择疑似异常（1）" }).click();
  await expect(panel.getByRole("checkbox", { name: "检测终端续接 终端异常账号" })).toBeChecked();
  await expect(
    panel.getByRole("checkbox", { name: "检测终端续接 终端正常账号" }),
  ).not.toBeChecked();
  expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await panel.getByRole("button", { name: "查看 终端异常账号 的终端续接详情" }).click();
  const detail = page.getByRole("dialog", { name: "终端续接检测详情" });
  await expect(detail).toContainText("没有可用的终端");
  await page.screenshot({ path: test.info().outputPath("terminal-continuity.png") });
});
