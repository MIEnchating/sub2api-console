import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("多账号检测默认 Astra，可手动输入并提交其他模型且不自动读取目录", async ({ page }) => {
  let modelRequests = 0;
  const requests: unknown[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.startsWith("/api/model-checks/animations/accounts/")) {
      modelRequests++;
      await route.fulfill({ status: 503, json: { detail: "不应读取模型列表" } });
      return;
    }
    if (path === "/api/model-checks/animations" && route.request().method() === "POST") {
      requests.push(route.request().postDataJSON());
      await route.fulfill({ status: 503, json: { detail: "隔离测试停止执行" } });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "动画模型测试" },
      "/api/accounts": ["41", "42"].map((id) => ({
        ...account,
        id,
        name: `动画账号${id}`,
        platform: "openai",
      })),
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("checkbox", { name: "检测 动画账号41" }).check();
  await page.getByRole("checkbox", { name: "检测 动画账号42" }).check();
  await expect(page.getByRole("button", { name: "获取模型", exact: true })).toBeEnabled();
  await expect(page.getByRole("combobox", { name: "检测模型" })).toHaveValue("gpt-6-astra");
  await page.getByRole("combobox", { name: "检测模型" }).fill("selected-model");
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "开始检测（2 个账号）" }).click();
  await expect.poll(() => requests.length).toBe(1);
  expect(requests[0]).toMatchObject({
    targets: [
      { account_id: "41", model: "selected-model" },
      { account_id: "42", model: "selected-model" },
    ],
  });
  await expect(page.getByText("隔离测试停止执行", { exact: true }).first()).toBeVisible();
  expect(modelRequests).toBe(0);
});
