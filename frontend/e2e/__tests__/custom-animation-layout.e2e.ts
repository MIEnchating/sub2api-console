import { expect, test } from "@playwright/test";
import type { Task } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("多条自定义动画记录分页展示且不撑高面板，较矮窗口仍能滚动访问完整内容", async ({
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
  await page.goto("/model-check");
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  await page.getByRole("tab", { name: "自定义接口", exact: true }).click();
  const content = page.getByRole("region", { name: "自定义动画检测内容", exact: true });
  const mobile = page.viewportSize()!.width < 768;
  const next = content.getByRole("button", { name: "下一页记录" });
  const previous = content.getByRole("button", { name: "上一页记录" });
  await expect(content.getByRole("article")).toHaveCount(mobile ? 1 : 3);
  await expect(previous).toBeDisabled();
  const firstModel = await content.getByRole("article").first().getAttribute("aria-label");
  expect(
    await content.evaluate(
      (element) =>
        element.scrollHeight <= element.clientHeight && element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true);
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
