import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";

test("账号快捷入口预选正确账号，完成模型检测后返回账号管理回显统计", async ({ page }) => {
  let completed = false;
  const window = { score: 20.7, samples: 1, passed: 1, failed: 0, inconclusive: 0 };
  const at = "2026-09-18T00:00:00Z";
  const quality = { short: window, long: window, evaluated_at: at };
  const rows = [
    { ...account, id: "40", name: "其他账号" },
    {
      ...account,
      id: "41",
      name: "联动账号",
      stability: { ...quality, short: { ...window, score: 100 }, long: { ...window, score: 100 } },
    },
  ];
  const task = {
    id: "check-41",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "succeeded",
    progress: 100,
    message: "检测完成",
    created_at: at,
    updated_at: at,
    result: {
      tests: [{ account_id: "41", claimed_model: "gpt-5.6-sol", verdict: "SOL_CONSISTENT" }],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks" && route.request().method() === "POST") {
      expect(route.request().postDataJSON().account_ids).toEqual(["41"]);
      completed = true;
      await route.fulfill({ json: task });
      return;
    }
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离测试" },
      "/api/accounts": rows,
      "/api/groups": [],
      "/api/policy": { advanced_policy: { manual_priority: { reserved_max: 10 } } },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: ["gpt-5.6-sol"] },
      "/api/model-checks/animations": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/account-statuses": completed
        ? [
            {
              account_id: "41",
              status: "consistent",
              checked_at: at,
              task_id: task.id,
              confidence: quality,
            },
          ]
        : [],
      "/api/accounts/41/models": { models: ["gpt-5.6-sol"] },
      "/api/tasks/check-41": task,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path.startsWith("/api/dictionaries")) await route.fulfill({ json: { items: [] } });
    else if (path in responses) await route.fulfill({ json: responses[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/accounts");
  const row = page.locator("tbody tr").filter({ hasText: "联动账号" });
  await row.getByRole("button", { name: "模型检测", exact: true }).click();
  await expect(page).toHaveURL(/\/model-check\?account_id=41/);
  await expect(page.getByRole("checkbox", { name: /联动账号/ })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: /选择账号 其他账号/ })).toHaveCount(0);
  await expect(page.getByText("联动账号（#41）", { exact: true })).toHaveCount(0);
  await page.goto("/animation-check?account_id=41");
  await expect(page.getByRole("article", { name: "账号 联动账号" })).toBeVisible();
  await expect(page.getByRole("article", { name: "账号 其他账号" })).toHaveCount(0);
  await page.getByRole("button", { name: "查看全部账号" }).click();
  await expect(page.getByRole("article", { name: "账号 其他账号" })).toBeVisible();
  await page.goto("/model-check");
  await page.getByRole("checkbox", { name: "联动账号 ID 41" }).click();
  await page.getByRole("checkbox", { name: /gpt-5.6-sol/ }).click();
  await page.getByRole("button", { name: /开始检测/ }).click();
  await expect(page.getByRole("button", { name: "查看账号 41 检测统计" })).toHaveText("符合特征");
  await page.getByRole("button", { name: "查看账号 41 检测统计" }).hover();
  await expect(page.getByLabel("置信度短期：20.7%")).toBeVisible();
  await page.getByRole("button", { name: "账号管理", exact: true }).click();
  await expect(page).toHaveURL(/\/accounts$/);
  await expect(page.getByLabel("置信度长期：20.7%")).toBeVisible();
  await expect(page.getByLabel("稳定性短期：100.0%")).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "置信度 / 稳定性" })).toBeVisible();
});
