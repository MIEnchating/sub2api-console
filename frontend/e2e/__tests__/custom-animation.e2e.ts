import { expect, test } from "@playwright/test";
import type { Task } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("无账号时自定义接口可确认生成、查看动画并清除 Key，长地址和模型在窄屏不溢出", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const model = "custom-model-".repeat(18);
  const baseURL = "https://custom.example.invalid/" + "proxy/".repeat(24) + "v1";
  let created = false;
  const task: Task = {
    id: "custom-e2e",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "succeeded",
    progress: 100,
    message: "动画检测完成",
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
    result: {
      account_ids: ["custom-e2e-target"],
      animations: [
        {
          account_id: "custom-e2e-target",
          account_name: "自定义接口",
          model,
          request_id: "custom-e2e-request",
          status: "succeeded",
          duration_ms: 1000,
          completed_at: "2026-09-14T00:00:00Z",
          svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><rect width="640" height="400" fill="#ffffff"/><circle cx="320" cy="200" r="60" fill="#047857"><animate attributeName="r" values="60;80;60" dur="2s" repeatCount="indefinite"/></circle></svg>',
        },
      ],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/animations/models" && route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({
        base_url: baseURL,
        api_key: "isolated-custom-key",
        platform: "anthropic",
      });
      await route.fulfill({ json: { models: [model] } });
      return;
    }
    if (path === "/api/model-checks/animations" && route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({
        targets: [],
        timeout_seconds: 120,
        custom: { base_url: baseURL, api_key: "isolated-custom-key", model, platform: "anthropic" },
      });
      created = true;
      await route.fulfill({ json: task });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "自定义动画测试" },
      "/api/accounts": [],
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": created ? [{ ...task, result: {} }] : [],
      "/api/tasks/custom-e2e": task,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/model-check");
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  await page.getByRole("tab", { name: "自定义接口", exact: true }).click();
  const panel = page.getByRole("tabpanel", { name: "自定义接口", exact: true });
  await panel.getByRole("combobox", { name: "接口类型" }).click();
  await page.getByRole("option", { name: "Anthropic" }).click();
  await panel.getByRole("textbox", { name: "Base URL" }).fill(baseURL);
  await panel.getByLabel("API Key").fill("isolated-custom-key");
  await panel.getByRole("button", { name: "获取模型" }).click();
  await expect(page.getByRole("option", { name: model, exact: true })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("custom-animation-model-list.png") });
  await page.getByRole("option", { name: model, exact: true }).click();
  await expect(panel.getByRole("combobox", { name: "检测模型" })).toHaveValue(model);
  const start = panel.getByRole("button", { name: "开始检测", exact: true });
  await expect(start).toHaveCSS("height", "32px");
  await start.click();
  const confirmation = page.getByRole("dialog", { name: "确认自定义接口检测" });
  await expect(confirmation).toContainText(baseURL);
  await expect(confirmation).not.toContainText("isolated-custom-key");
  expect(await confirmation.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(created).toBe(false);
  await confirmation.getByRole("button", { name: "确认并开始检测" }).click();
  await expect(panel.getByLabel("API Key")).toHaveValue("");
  const image = panel.getByRole("img", { name: /生成的/ });
  await image.scrollIntoViewIfNeeded();
  await expect(image).toHaveJSProperty("complete", true);
  expect(await image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBeGreaterThan(
    0,
  );
  expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("custom-animation.png") });
  await panel.getByRole("button", { name: "放大查看 自定义接口 的动画" }).click();
  await expect(page.getByRole("dialog", { name: "动画预览" })).toBeVisible();
  await page.keyboard.press("Escape");
  await panel.getByLabel("API Key").fill("temporary-key");
  await page.getByRole("tab", { name: "常规检测", exact: true }).click();
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  await expect(panel.getByLabel("API Key")).toHaveValue("");
  await page.reload();
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  await page.getByRole("tab", { name: "自定义接口", exact: true }).click();
  await expect(panel.getByRole("img", { name: /生成的/ })).toHaveCount(1);
  await test.step("桌面表单与结果适应面板，窄屏可滚动到达操作与完整结果", async () => {
    const content = panel.getByRole("region", { name: "自定义动画检测内容", exact: true });
    await expect(content).not.toHaveCSS("scrollbar-width", "none");
    await expect(content).toHaveCSS("overflow-y", "auto");
    if (page.viewportSize()!.width >= 640) {
      expect(
        await content.evaluate((element) => element.scrollHeight <= element.clientHeight),
      ).toBe(true);
    }
    const start = panel.getByRole("button", { name: "开始检测", exact: true });
    await start.scrollIntoViewIfNeeded();
    await expect(start).toBeInViewport({ ratio: 1 });
    const duration = panel.getByText("耗时 1.0 秒", { exact: true });
    await duration.scrollIntoViewIfNeeded();
    await expect(duration).toBeInViewport({ ratio: 1 });
    await page.screenshot({ path: testInfo.outputPath("custom-animation-fitted.png") });
  });
});
