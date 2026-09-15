import { expect, test } from "@playwright/test";

import type { SystemMetrics, Task, TaskSummary } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

const longMessage = "UpstreamRequestInspectionPendingWithoutWhitespace".repeat(18);
const longOperation = "automatic-inspection-with-an-unrecognized-operation-name".repeat(6);
const activeTask: TaskSummary = {
  id: "system-layout-running-task-with-a-long-stable-identifier",
  skill: "sub2api-connectivity-test",
  operation: longOperation,
  status: "running",
  progress: 37.5,
  message: longMessage,
  created_at: "2026-09-15T08:00:00Z",
  updated_at: "2026-09-15T08:01:00Z",
  system_info: true,
};
const historyTask: TaskSummary = {
  ...activeTask,
  id: "system-layout-completed",
  operation: "active-probe",
  status: "succeeded",
  progress: 100,
  message: "已完成模型探活",
};
const metrics: SystemMetrics = {
  sampled_at: "2026-09-15T08:01:00Z",
  cpu: { usage_percent: 37.5, logical_cores: 8 },
  memory: {
    usage_percent: 62.5,
    used_bytes: 10 * 1024 ** 3,
    available_bytes: 6 * 1024 ** 3,
    total_bytes: 16 * 1024 ** 3,
  },
  disk: {
    usage_percent: 40,
    used_bytes: 80 * 1024 ** 3,
    available_bytes: 120 * 1024 ** 3,
    total_bytes: 200 * 1024 ** 3,
  },
};

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "系统信息布局测试" },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/dictionaries": { items: [] },
      "/api/tasks": [activeTask, historyTask],
      "/api/system/metrics": metrics,
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (route.request().method() === "GET" && path.startsWith("/api/tasks/")) {
      const task: Task = { ...activeTask, id: path.slice("/api/tasks/".length), result: {} };
      await route.fulfill({ json: task });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("窄屏资源卡片保持三列，资源数值和说明完整留在卡片内", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 720 });
  await page.goto("/system-info");
  const resources = page.getByLabel("服务器资源占用");
  await expect(resources.getByRole("progressbar")).toHaveCount(3);
  const cards = resources.locator('[data-slot="card"]');
  await expect(cards).toHaveCount(3);
  const bounds = await cards.evaluateAll((elements) =>
    elements.map((element) => ({
      y: element.getBoundingClientRect().y,
      x: element.getBoundingClientRect().x,
      width: element.getBoundingClientRect().width,
      overflows: element.scrollWidth > element.clientWidth,
    })),
  );
  expect(bounds[1]!.y).toBe(bounds[0]!.y);
  expect(bounds[2]!.y).toBe(bounds[0]!.y);
  expect(bounds[1]!.x).toBeGreaterThanOrEqual(bounds[0]!.x + bounds[0]!.width);
  expect(bounds[2]!.x).toBeGreaterThanOrEqual(bounds[1]!.x + bounds[1]!.width);
  expect(bounds.map((card) => card.overflows)).toEqual([false, false, false]);
  await expect(resources.getByText("10 GB / 16 GB", { exact: true })).toBeVisible();
  await expect(resources.getByText("80 GB / 200 GB", { exact: true })).toBeVisible();
});

test("超长任务消息最多两行并保留独立百分比，辅助信息使用较小字号", async ({ page }) => {
  await page.goto("/system-info");
  const table = page.getByRole("table");
  const message = table.getByText(longMessage, { exact: true });
  await expect(message).toHaveCSS("-webkit-line-clamp", "2");
  await expect(message).toHaveCSS("overflow", "hidden");
  await expect(message).toHaveCSS("white-space", "normal");
  await expect(message).toHaveCSS("font-size", "14px");
  const percent = table.getByText("37.5%", { exact: true });
  const messageBounds = (await message.boundingBox())!;
  const percentBounds = (await percent.boundingBox())!;
  expect(messageBounds.x + messageBounds.width).toBeLessThanOrEqual(percentBounds.x);
  expect(await message.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  );
  await expect(table.getByText(activeTask.id, { exact: true })).toHaveCSS("font-size", "12px");
  await expect(table.getByText(longOperation, { exact: true })).toHaveCSS("font-size", "14px");
});

test("超长任务类型在固定列内截断，横向溢出仅发生在任务表格内", async ({ page, viewport }) => {
  await page.goto("/system-info");
  const table = page.getByRole("table", { name: "后台任务" });
  const operation = table.getByText(longOperation, { exact: true });
  await expect(table).toHaveCSS("table-layout", "fixed");
  await expect(table).toHaveCSS("min-width", "760px");
  await expect(operation).toHaveCSS("text-overflow", "ellipsis");
  expect(await operation.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
    true,
  );
  const container = page.getByTestId("system-tasks-panel").locator('[data-slot="table-container"]');
  await expect(container).toHaveCSS("overflow-x", "auto");
  if (viewport!.width < 760) {
    expect(await container.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
      true,
    );
  }
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("system-info.png"), fullPage: true });
});

test("窄屏分类入口等宽可见，状态筛选可换行且不撑宽任务面板", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 720 });
  await page.goto("/system-info");
  const panel = page.getByTestId("system-tasks-panel");
  const tabs = panel.getByRole("tablist", { name: "任务分类" });
  const active = tabs.getByRole("tab", { name: "进行中任务 1" });
  const history = tabs.getByRole("tab", { name: "历史任务 1" });
  await expect(active).toBeInViewport({ ratio: 1 });
  await expect(history).toBeInViewport({ ratio: 1 });
  const activeBounds = (await active.boundingBox())!;
  const historyBounds = (await history.boundingBox())!;
  expect(Math.abs(activeBounds.width - historyBounds.width)).toBeLessThanOrEqual(1);
  expect(await tabs.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(panel.getByRole("button", { name: "任务状态筛选" })).toBeInViewport({ ratio: 1 });
  expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
});

test("键盘切换任务分类时同步选中状态并关联当前任务列表", async ({ page }) => {
  await page.goto("/system-info");
  const tabs = page.getByRole("tablist", { name: "任务分类" });
  const active = tabs.getByRole("tab", { name: "进行中任务 1" });
  const history = tabs.getByRole("tab", { name: "历史任务 1" });
  await active.focus();
  await active.press("ArrowRight");
  await expect(history).toBeFocused();
  await expect(history).toHaveAttribute("aria-selected", "true");
  await expect(active).toHaveAttribute("aria-selected", "false");
  const panel = page.getByRole("tabpanel", { name: "历史任务 1" });
  await expect(panel.getByText(historyTask.message, { exact: true })).toBeVisible();
  await expect(history).toHaveAttribute("aria-controls", (await panel.getAttribute("id"))!);
  await expect(panel.getByRole("tablist")).toHaveCount(0);
  await history.press("Home");
  await expect(active).toBeFocused();
  await expect(active).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("tabpanel", { name: "进行中任务 1" })).toBeVisible();
});

test("低高度窗口任务表内部滚动后仍能查看末条任务并关闭完整换行的详情", async ({
  page,
  viewport,
}) => {
  const tasks = Array.from({ length: 18 }, (_, index): TaskSummary => ({
    ...activeTask,
    id: `system-layout-task-${index + 1}`,
    operation: "active-probe",
    message: `正在执行任务 ${index + 1}`,
  }));
  await page.route("**/api/tasks?*", (route) => route.fulfill({ json: tasks }));
  await page.setViewportSize({ width: viewport!.width, height: 480 });
  await page.goto("/system-info");
  const table = page.getByRole("table", { name: "后台任务" });
  await expect(table.getByRole("button", { name: "查看任务", exact: true })).toHaveCount(18);
  const container = page.getByTestId("system-tasks-panel").locator('[data-slot="table-container"]');
  await expect(container).toHaveCSS("overflow-y", "auto");
  await container.scrollIntoViewIfNeeded();
  await container.evaluate((element) =>
    element.scrollTo(element.scrollWidth, element.scrollHeight),
  );
  await expect.poll(() => container.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  const details = table.getByRole("button", { name: "查看任务", exact: true }).last();
  await details.scrollIntoViewIfNeeded();
  await expect(details).toBeInViewport({ ratio: 1 });
  await details.click();
  const dialog = page.getByRole("dialog", { name: "任务详情" });
  const message = dialog.getByText(longMessage, { exact: true });
  await expect(message).toBeVisible();
  expect(await message.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const close = dialog.getByRole("button", { name: "关闭", exact: true }).last();
  await expect(close).toBeInViewport({ ratio: 1 });
  await close.click();
  await expect(dialog).not.toBeVisible();
  await expect(details).toBeFocused();
});

test("窄屏没有任务时无需横向滚动即可完整看到空状态", async ({ page }) => {
  await page.route("**/api/tasks?*", (route) => route.fulfill({ json: [] }));
  await page.setViewportSize({ width: 320, height: 480 });
  await page.goto("/system-info");
  const empty = page.getByText("当前没有进行中的任务", { exact: true });
  await empty.scrollIntoViewIfNeeded();
  await expect(empty).toBeInViewport({ ratio: 1 });
  expect(await empty.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const container = page.getByTestId("system-tasks-panel").locator('[data-slot="table-container"]');
  expect(await container.evaluate((element) => element.scrollLeft)).toBe(0);
});
