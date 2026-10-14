import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { account } from "../../src/features/accounts/__tests__/fixtures";

test("新模型可直接检测并查看独立身份与能力证据", async ({ page }) => {
  const task = {
    id: "verification-task",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "succeeded",
    progress: 100,
    message: "完成",
    created_at: "2026-10-14T00:00:00Z",
    updated_at: "2026-10-14T00:01:00Z",
    result: {
      tests: [
        {
          account_id: "41",
          account_name: "检测账号",
          claimed_model: "new-model",
          checker: "verification",
          verdict: "NO_ANOMALIES",
          authenticity: { status: "unverified", summary: "第三方身份尚未证实" },
          capability: { status: "passed", summary: "本次能力检测通过" },
          total: 7,
          passed: 7,
          evidence: [{ id: "source", label: "请求来源", detail: "第三方接口" }],
          probes: [
            {
              id: "arithmetic",
              label: "多步计算",
              request_id: "req-1",
              round: 1,
              status: "passed",
              prompt: "7 × 3",
              expected: "21",
              answer: "**21**",
              protocol: "openai_responses",
              http_status: 200,
            },
          ],
        },
      ],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    expect(path).not.toContain("model-checks/configuration");
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/accounts": [{ ...account, id: "41", name: "检测账号", platform: "openai" }],
      "/api/accounts/41/models": { models: ["new-model"] },
      "/api/model-checks": task,
      "/api/tasks/verification-task": task,
      "/api/model-checks/account-statuses": [],
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ status: 200, contentType: "text/event-stream", body: "" });
      return;
    }
    await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/model-check");
  await expect(page.getByRole("button", { name: "检测规则与题库" })).toHaveCount(0);
  await page.getByRole("button", { name: "全选账号" }).click();
  await page.getByRole("checkbox", { name: "选择模型 new-model" }).click();
  await expect(page.getByText(/计划 7 项检测/)).toBeVisible();
  await page.getByRole("button", { name: /开始检测/ }).click();
  const result = page.getByRole("dialog", { name: "模型检测结果", exact: true });
  await expect(result).toBeVisible();
  await expect(result.getByText("身份未证实").filter({ visible: true })).toBeVisible();
  await result.getByRole("button", { name: "查看检测证据" }).filter({ visible: true }).click();
  const evidence = page.getByRole("dialog", { name: "检测证据", exact: true });
  await expect(evidence.getByText("**21**", { exact: true })).toBeVisible();
  await expect(evidence.getByText(/HTTP 200/)).toBeVisible();
  await expect(evidence).toBeInViewport();
  await page.keyboard.press("Escape");
  await expect(evidence).toHaveCount(0);
  await expect(result).toBeVisible();
});
