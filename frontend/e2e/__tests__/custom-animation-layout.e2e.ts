import { expect, test } from "@playwright/test";
import type { Task } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("自定义接口分区排列，模型输入可读且空记录与操作栏不溢出", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/accounts": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [],
      "/api/model-checks/timeouts/animation": { timeout_seconds: 120, version: 0 },
      "/api/model-checks/timeouts/precheck": { timeout_seconds: 120, version: 0 },
      "/api/dictionaries": { items: [] },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("tab", { name: "自定义接口", exact: true }).click();
  await expect(page.getByRole("spinbutton", { name: "动画超时（秒）" })).toHaveValue("120");
  await expect(page.getByRole("spinbutton", { name: "前置检测超时（秒）" })).toHaveValue("120");
  const model = page.getByRole("combobox", { name: "检测模型" });
  expect((await model.boundingBox())!.width).toBeGreaterThanOrEqual(180);
  const connection = page.getByRole("group", { name: "接口配置", exact: true });
  const operations = page.getByRole("group", { name: "自定义接口检测操作", exact: true });
  const parameters = operations.getByRole("group", { name: "检测参数", exact: true });
  const connectionBox = (await connection.boundingBox())!;
  const parametersBox = (await parameters.boundingBox())!;
  const operationsBox = (await operations.boundingBox())!;
  expect(parametersBox.y).toBeGreaterThan(connectionBox.y + connectionBox.height);
  expect(operationsBox.y).toBeGreaterThan(connectionBox.y + connectionBox.height);
  if (page.viewportSize()!.width >= 1440) {
    const startBox = (await operations
      .getByRole("button", { name: "开始检测", exact: true })
      .boundingBox())!;
    expect(startBox.y).toBeGreaterThanOrEqual(parametersBox.y + parametersBox.height);
    expect(startBox.y).toBeCloseTo(
      (await operations.getByRole("button", { name: "清空配置", exact: true }).boundingBox())!.y,
      0,
    );
  }
  const empty = page.getByRole("group", { name: "暂无检测记录", exact: true });
  expect((await empty.boundingBox())!.height).toBeLessThanOrEqual(180);
  await expect(operations.getByRole("button", { name: "编辑动画提示词" })).toBeVisible();
  await expect(page.getByText("暂无自定义接口检测记录")).toBeVisible();
  const foreground = await page.evaluate(() => {
    const sample = document.createElement("span");
    sample.style.color = "var(--foreground)";
    document.body.append(sample);
    const color = getComputedStyle(sample).color;
    sample.remove();
    return color;
  });
  for (const name of ["账号管理", "编辑动画提示词", "清空配置"]) {
    await expect(page.getByRole("button", { name, exact: true })).toHaveCSS("color", foreground);
  }
  await expect(operations.getByRole("button", { name: "开始检测", exact: true })).toHaveAttribute(
    "data-variant",
    "default",
  );

  for (const button of await operations.getByRole("button").all()) {
    await expect(button).toHaveCSS("height", "32px");
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("custom-interface-form.png") });
});

test("多条自定义动画记录分页展示且无横向溢出，较矮窗口仍能滚动访问完整内容", async ({
  page,
}, testInfo) => {
  const tasks: Task[] = [1, 2, 3, 4].map((index) => ({
    id: `layout-${index}`,
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "succeeded",
    progress: 100,
    message: "动画检测完成",
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
    result: {
      account_ids: [`custom-layout-${index}`],
      animations: [
        {
          account_id: `custom-layout-${index}`,
          account_name: "自定义接口",
          model: `layout-model-${index}`,
          request_id: `layout-request-${index}`,
          status: "succeeded",
          duration_ms: 1000,
          completed_at: "2026-09-14T00:00:00Z",
          svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><rect width="640" height="400" fill="#047857"/></svg>',
        },
      ],
    },
  }));
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/accounts": [],
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": tasks,
      ...Object.fromEntries(tasks.map((task) => [`/api/tasks/${task.id}`, task])),
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("tab", { name: "自定义接口", exact: true }).click();
  const content = page.getByRole("region", { name: "自定义动画检测内容", exact: true });
  const mobile = page.viewportSize()!.width < 768;
  const next = content.getByRole("button", { name: "下一页记录" });
  const previous = content.getByRole("button", { name: "上一页记录" });
  await expect(content.getByRole("article")).toHaveCount(mobile ? 1 : 3);
  await expect(previous).toBeDisabled();
  const firstModel = await content.getByRole("article").first().getAttribute("aria-label");
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await next.click();
  await expect(content.getByRole("article").first()).not.toHaveAttribute("aria-label", firstModel!);
  await expect(previous).toBeEnabled();
  if (!mobile) await expect(next).toBeDisabled();
  await previous.focus();
  await page.keyboard.press("Enter");
  await expect(content.getByRole("article").first()).toHaveAttribute("aria-label", firstModel!);
  await page.screenshot({ path: testInfo.outputPath("custom-animation-pagination.png") });
  await page.setViewportSize({ width: page.viewportSize()!.width, height: 500 });
  await expect(content).not.toHaveCSS("scrollbar-width", "none");
  await content.getByText("耗时 1.0 秒", { exact: true }).first().scrollIntoViewIfNeeded();
  await expect(content.getByText("耗时 1.0 秒", { exact: true }).first()).toBeInViewport({
    ratio: 1,
  });
});
