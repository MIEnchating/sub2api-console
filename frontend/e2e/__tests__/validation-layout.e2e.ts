import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("登录表单从空值校验到错误清除时，居中的表单及按钮保持原位", async ({ page }) => {
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/setup/status")
      return route.fulfill({ json: { initialized: true, configuration_errors: [] } });
    if (path === "/api/auth/session") return route.fulfill({ json: { authenticated: false } });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/");
  const username = page.getByRole("textbox", { name: "账号", exact: true });
  const submit = page.getByRole("button", { name: "登录", exact: true });
  await expect(username).toBeVisible();
  const form = page.locator("form");
  const initial = await form.boundingBox();
  const buttonBox = await submit.boundingBox();
  await submit.click();
  await expect(username).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByRole("alert")).toHaveCount(2);
  expect(await form.boundingBox()).toEqual(initial);
  expect(await submit.boundingBox()).toEqual(buttonBox);
  await username.fill("layout-user");
  await page.getByLabel("密码", { exact: true }).fill("layout-test-password");
  await expect(page.getByRole("alert")).toHaveCount(0);
  expect(await form.boundingBox()).toEqual(initial);
});

test("个人信息多字段校验及错误换行时，字段和保存按钮相对表单的位置不变", async ({ page }) => {
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "layout-user" },
    };
    if (path in fixtures) return route.fulfill({ json: fixtures[path] });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/profile");
  const form = page.locator("form");
  const username = page.getByRole("textbox", { name: "账号", exact: true });
  await expect(username).toHaveValue("layout-user");
  const positions = (): Promise<number[]> =>
    form.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      return [
        bounds.width,
        bounds.height,
        ...Array.from(element.querySelectorAll("input,button")).flatMap((child) => {
          const box = child.getBoundingClientRect();
          return [box.x - bounds.x, box.y - bounds.y, box.width, box.height];
        }),
      ].map((value) => Math.round(value));
    });
  const initial = await positions();
  await username.fill("x");
  await page.getByLabel("新密码（可选）", { exact: true }).fill("123");
  await page.getByRole("button", { name: "保存修改", exact: true }).click();
  await expect(username).toHaveAttribute("aria-invalid", "true");
  expect(await positions()).toEqual(initial);
  await username.fill("a".repeat(81));
  await expect(page.getByRole("alert").filter({ hasText: "账号不能超过 80 个字符" })).toBeVisible();
  expect(await positions()).toEqual(initial);
  await username.fill("layout-user");
  await expect(username).toHaveAttribute("aria-invalid", "false");
  expect(await positions()).toEqual(initial);
});

test("动画检测校验、账号计数和模型读取状态变化时列表与操作按钮保持原位", async ({ page }) => {
  let releaseModels: () => void = () => {};
  const modelsReady = new Promise<void>((resolve) => {
    releaseModels = resolve;
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (/^\/api\/accounts\/\d+\/models$/.test(path)) {
      if (path === "/api/accounts/41/models") await modelsReady;
      await route.fulfill({ json: { models: [] } });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/accounts": Array.from({ length: 20 }, (_, index) => ({
        ...account,
        id: String(41 + index),
        name: `布局账号 ${index + 1}`,
        platform: "openai",
      })),
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/model-check");
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  const region = page.getByRole("region", { name: "动画账号卡片", exact: true });
  const first = page.getByRole("checkbox", { name: /^检测 布局账号 1\b/ });
  await expect(first).toBeVisible();
  const start = page.getByRole("button", { name: /开始检测/ });
  const operations = page.getByRole("group", { name: "动画检测操作", exact: true });
  // Empty feedback must not reserve a third toolbar row.
  expect((await operations.boundingBox())!.height).toBeLessThanOrEqual(58);
  await expect(page.locator('[data-slot="animation-settings-feedback"]')).toHaveCount(0);
  const regionBox = await region.boundingBox();
  const startBox = await start.boundingBox();
  await first.check();
  await first.uncheck();
  await expect(start).toBeDisabled();
  expect(await region.boundingBox()).toEqual(regionBox);
  await first.check();
  await expect(page.getByRole("button", { name: /开始检测/ })).toBeEnabled();
  await start.click();
  await expect(page.getByRole("combobox", { name: "检测模型" })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(await region.boundingBox()).toEqual(regionBox);
  await page.getByRole("combobox", { name: "检测模型" }).fill("fixture-model");
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
  await expect(page.getByRole("status", { name: "正在读取共同模型" })).toBeVisible();
  expect(await region.boundingBox()).toEqual(regionBox);
  releaseModels();
  await expect(page.getByRole("button", { name: "获取模型", exact: true })).toBeEnabled();
  await expect(page.getByRole("combobox", { name: "检测模型" })).toHaveValue("fixture-model");
  await expect(page.locator("#animation-common-models option")).toHaveCount(0);
  expect(await region.boundingBox()).toEqual(regionBox);
  await page.getByRole("button", { name: "选择前 20 个账号" }).click();
  await expect(start).toHaveText("开始检测（20 个账号）");
  expect(await start.boundingBox()).toEqual(startBox);
  await page.screenshot({ path: test.info().outputPath("validation-stable.png"), fullPage: true });
});
