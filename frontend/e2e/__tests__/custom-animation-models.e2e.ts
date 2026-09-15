import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "模型列表测试" },
      "/api/accounts": [],
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
  await page.goto("/model-check");
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  await page.getByRole("tab", { name: "自定义接口", exact: true }).click();
  await page.getByRole("textbox", { name: "Base URL" }).fill("https://models.example.invalid/v1");
  await page.getByLabel("API Key").fill("isolated-list-key");
});

test("未填写模型时可获取列表并用键盘选择，获取按钮保持标准尺寸", async ({ page }) => {
  await page.route("**/api/model-checks/animations/models", async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().postDataJSON()).toEqual({
      base_url: "https://models.example.invalid/v1",
      api_key: "isolated-list-key",
      platform: "openai",
    });
    await route.fulfill({ json: { models: ["alpha-model", "beta-model"] } });
  });
  const panel = page.getByRole("tabpanel", { name: "自定义接口", exact: true });
  const button = panel.getByRole("button", { name: "获取模型" });
  await expect(button).toHaveCSS("height", "32px");
  await button.click();
  await expect(page.getByRole("option", { name: "alpha-model" })).toBeVisible();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(panel.getByRole("combobox", { name: "检测模型" })).toHaveValue("alpha-model");
  await panel.getByRole("combobox", { name: "检测模型" }).fill("beta");
  await expect(page.getByRole("option", { name: "beta-model" })).toBeVisible();
  await expect(page.getByRole("option", { name: "alpha-model" })).toHaveCount(0);
});

test("读取失败时保留手填模型并允许重试", async ({ page }) => {
  let failed = true;
  await page.route("**/api/model-checks/animations/models", async (route) => {
    if (failed) await route.fulfill({ status: 502, json: { detail: "Key 无效，请检查后重试" } });
    else await route.fulfill({ json: { models: ["manual-model"] } });
  });
  const model = page.getByRole("combobox", { name: "检测模型" });
  await model.fill("manual-model");
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
  await expect(page.getByText("Key 无效，请检查后重试", { exact: true })).toBeVisible();
  await expect(model).toHaveValue("manual-model");
  failed = false;
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
  await expect(page.getByRole("option", { name: "manual-model" })).toBeVisible();
});

test("修改 Key 会取消正在读取的旧列表，新请求只展示新凭据的模型", async ({ page }) => {
  let release: () => void = () => {};
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/model-checks/animations/models", async (route) => {
    if (route.request().postDataJSON().api_key === "isolated-list-key") {
      await pending;
      await route.fulfill({ json: { models: ["stale-model"] } }).catch(() => {});
    } else await route.fulfill({ json: { models: ["current-model"] } });
  });
  const aborted = page.waitForEvent("requestfailed", {
    predicate: (request) => request.url().endsWith("/animations/models"),
  });
  const button = page.getByRole("button", { name: "获取模型", exact: true });
  await button.click();
  await expect(button).toBeDisabled();
  await expect(page.getByRole("status", { name: "正在读取模型" })).toBeVisible();
  await page.getByLabel("API Key").fill("replacement-list-key");
  await aborted;
  await expect(button).toBeEnabled();
  await button.click();
  await expect(page.getByRole("option", { name: "current-model" })).toBeVisible();
  release();
  await expect(page.getByRole("option", { name: "stale-model" })).toHaveCount(0);
});

test("上游返回空列表时仍允许手动输入模型", async ({ page }) => {
  await page.route("**/api/model-checks/animations/models", async (route) => {
    await route.fulfill({ json: { models: [] } });
  });
  const button = page.getByRole("button", { name: "获取模型", exact: true });
  await button.click();
  await expect(page.getByText("暂无匹配模型", { exact: true })).toBeVisible();
  const model = page.getByRole("combobox", { name: "检测模型" });
  await model.fill("manual-model");
  await expect(model).toHaveValue("manual-model");
});

test("Key 为空时展示字段错误并阻止模型列表请求", async ({ page }) => {
  let calls = 0;
  await page.route("**/api/model-checks/animations/models", async (route) => {
    calls++;
    await route.fulfill({ json: { models: [] } });
  });
  await page.getByLabel("API Key").clear();
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
  await expect(page.getByText("请输入 API Key", { exact: true })).toBeVisible();
  expect(calls).toBe(0);
});
