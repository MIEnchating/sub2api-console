import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";

import type { AutoInspectionStatus } from "../../src/api";
import { inspection } from "./fixtures/operations";
import { pageFixtures } from "./fixtures/page-shell";

const longOperationLabel = `主动探测-${"very-long-operation-name-".repeat(7)}`;
const longCycle = `继承账号策略-${"very-long-scheduling-cycle-".repeat(7)}`;

const layoutInspection: AutoInspectionStatus = {
  ...inspection,
  queue: [
    {
      ...inspection.queue[0],
      operations: [
        {
          ...inspection.queue[0].operations[0],
          label: longOperationLabel,
          cycle: longCycle,
        },
      ],
    },
  ],
};

const scheduledInspection: AutoInspectionStatus = {
  ...inspection,
  queue: [
    {
      ...inspection.queue[0],
      target_count: 24,
      operations: [
        { operation: "upstream_sync", label: "上游数据同步", cycle: "每 5 分钟", due: true },
        { operation: "auth_recovery", label: "鉴权自动恢复", cycle: "鉴权失效时", due: false },
        {
          operation: "account_rate_sync",
          label: "账号倍率与名称同步",
          cycle: "每 10 分钟",
          due: true,
        },
        { operation: "traffic_refresh", label: "真实流量同步", cycle: "每次心跳", due: true },
        { operation: "active_probe", label: "主动探测", cycle: "继承账号探测策略", due: true },
        { operation: "routing_calculation", label: "调度计算", cycle: "每次心跳", due: true },
        { operation: "alert_evaluation", label: "告警检测", cycle: "继承告警策略", due: true },
      ].map((operation) => ({ ...operation, target_count: 24 })),
    },
  ],
  heartbeat_history: [
    {
      ...inspection.heartbeat_history[0],
      status: "succeeded",
      operations: ["upstream_sync", "active_probe", "routing_calculation", "alert_evaluation"],
      operation_timings: [{ operation: "active_probe", duration_seconds: 1 }],
      error: null,
    },
    {
      ...inspection.heartbeat_history[0],
      checked_at: "2026-09-08T23:59:00Z",
      error: "一个账号探测超时，请检查上游连接。",
    },
  ],
};

const busyInspection: AutoInspectionStatus = {
  ...inspection,
  queue: [
    {
      ...inspection.queue[0],
      operations: Array.from({ length: 18 }, (_, index) => ({
        ...inspection.queue[0].operations[0],
        operation: `layout-operation-${index + 1}`,
        label: `巡检操作 ${index + 1}`,
      })),
    },
  ],
  heartbeat_history: Array.from({ length: 30 }, (_, index) => ({
    ...inspection.heartbeat_history[0],
    checked_at: `2026-09-09T00:${String(index).padStart(2, "0")}:00Z`,
    completed_at: `2026-09-09T00:${String(index).padStart(2, "0")}:01Z`,
    error: `第 ${index + 1} 轮探测失败，请检查上游连接。`,
  })),
};

async function useInspection(page: Page, status: AutoInspectionStatus): Promise<void> {
  await page.route("**/api/inspection/automation", async (route) => {
    await route.fulfill({ json: status });
  });
}

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "巡检布局测试" },
      "/api/inspection/automation": layoutInspection,
      "/api/accounts": [],
      "/api/groups": [],
      "/api/tasks": [],
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

for (const width of [1280, 1440]) {
  test(`${width}px 桌面任务队列和心跳记录无需横向滚动且长操作名称与周期不溢出`, async ({
    page,
    viewport,
  }) => {
    test.skip(viewport!.width < 1280, "桌面双栏布局");
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/auto-inspection");
    const queue = page.getByTestId("auto-inspection-queue-scroll-area");
    const table = page.getByTestId("auto-inspection-heartbeat-table");
    await expect(queue.getByText(longOperationLabel, { exact: true })).toBeVisible();
    await expect(table.getByRole("button", { name: "查看心跳详情" })).toBeVisible();
    expect(await queue.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    expect(
      await table.evaluate((element) => {
        const container = element.closest('[data-slot="table-container"]');
        return container !== null && container.scrollWidth <= container.clientWidth;
      }),
    ).toBe(true);
    await expect(queue.getByText(longCycle)).toBeVisible();
    await table.getByRole("button", { name: "查看心跳详情" }).scrollIntoViewIfNeeded();
    await expect(table.getByRole("button", { name: "查看心跳详情" })).toBeInViewport({ ratio: 1 });
    await expect(table.getByRole("columnheader")).toHaveCount(5);
    const queueBounds = await queue.boundingBox();
    const tableBounds = await table.boundingBox();
    if (width === 1440) {
      expect(queueBounds!.x + queueBounds!.width).toBeLessThan(tableBounds!.x);
    }
    await expect(page.getByRole("heading", { name: "自动巡检", exact: true })).toBeInViewport({
      ratio: 1,
    });
    await expect(page.getByRole("button", { name: "保存自动巡检", exact: true })).toBeInViewport({
      ratio: 1,
    });
    expect(
      await page.getByText("巡检服务", { exact: true }).evaluate((element) => {
        const range = document.createRange();
        range.selectNodeContents(element);
        return range.getClientRects().length;
      }),
    ).toBe(1);
  });
}

test("手机上下排列数据区且心跳详情无需横向滚动即可打开，Escape 关闭后焦点返回入口", async ({
  page,
  viewport,
}) => {
  test.skip(viewport!.width >= 768, "手机布局与详情键盘交互");
  await page.goto("/auto-inspection");
  const queue = page.getByTestId("auto-inspection-queue-scroll-area");
  const table = page.getByRole("table", { name: "巡检心跳记录", exact: true });
  await expect(queue.getByText(longOperationLabel, { exact: true })).toBeVisible();
  const container = page.locator('[data-slot="table-container"]').filter({ has: table });
  await container.scrollIntoViewIfNeeded();
  const queueBounds = await queue.boundingBox();
  const tableBounds = await table.boundingBox();
  expect(queueBounds!.y + queueBounds!.height).toBeLessThan(tableBounds!.y);
  expect(await container.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
    true,
  );
  expect(await container.evaluate((element) => element.scrollLeft)).toBe(0);
  const trigger = table.getByRole("button", { name: "查看心跳详情", exact: true });
  await expect(trigger).toBeInViewport({ ratio: 1 });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "巡检心跳详情" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("测试探测失败");
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await expect(trigger).toBeFocused();
  await expect(trigger).toBeInViewport({ ratio: 1 });
  expect(await queue.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(
    await page
      .locator('[data-slot="page-content"]')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  await expect(page.getByRole("heading", { name: "自动巡检", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await expect(page.getByRole("button", { name: "保存自动巡检", exact: true })).toBeInViewport({
    ratio: 1,
  });
});

test("多条任务和历史可分别滚动至最后一项且心跳表头与页面标题保持可见", async ({
  page,
  viewport,
}) => {
  test.skip(viewport!.width < 1280, "桌面独立滚动区");
  await useInspection(page, busyInspection);
  await page.goto("/auto-inspection");
  const queue = page.getByRole("region", { name: "巡检任务队列", exact: true });
  await expect(queue.getByText("巡检操作 18", { exact: true })).toBeAttached();
  await queue.focus();
  await queue.press("End");
  await expect(queue.getByText("巡检操作 18", { exact: true })).toBeInViewport({ ratio: 1 });
  const table = page.getByRole("table", { name: "巡检心跳记录", exact: true });
  const container = page.locator('[data-slot="table-container"]').filter({ has: table });
  await container.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  await expect(
    table.getByRole("button", { name: "查看心跳详情", exact: true }).last(),
  ).toBeInViewport({
    ratio: 1,
  });
  await expect(table.getByRole("columnheader", { name: "检查时间", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await expect(page.getByRole("heading", { name: "自动巡检", exact: true })).toBeInViewport({
    ratio: 1,
  });
});

test("精简设置后仍可修改心跳并关闭巡检，保存成功后保留已确认的设置", async ({ page }) => {
  const saved = { ...scheduledInspection, enabled: false, interval_seconds: 30 };
  let status = scheduledInspection;
  await page.route("**/api/inspection/automation", async (route) => {
    if (route.request().method() !== "GET") status = saved;
    await route.fulfill({ json: status });
  });
  await page.goto("/auto-inspection");
  const enabled = page.getByRole("switch", { name: "启用自动巡检", exact: true });
  const interval = page.getByRole("spinbutton", { name: "调度心跳周期", exact: true });
  await expect(enabled).toBeChecked();
  await expect(
    page.getByRole("region", { name: "巡检任务队列" }).getByRole("listitem"),
  ).toHaveCount(7);
  await page.screenshot({ path: test.info().outputPath("auto-inspection-seven-steps.png") });
  if (page.viewportSize()!.width < 768) {
    const table = page.getByRole("table", { name: "巡检心跳记录", exact: true });
    await page
      .locator('[data-slot="table-container"]')
      .filter({ has: table })
      .scrollIntoViewIfNeeded();
    await page.screenshot({ path: test.info().outputPath("auto-inspection-heartbeat.png") });
  }
  await enabled.click();
  await interval.fill("30");
  const saveResponse = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === "/api/inspection/automation" &&
      response.request().method() !== "GET",
  );
  await page.getByRole("button", { name: "保存自动巡检", exact: true }).click();
  expect((await saveResponse).request().postDataJSON()).toEqual({
    enabled: false,
    interval_seconds: 30,
  });
  await expect(page.getByText("自动巡检已关闭", { exact: true })).toBeVisible();
  await expect(enabled).not.toBeChecked();
  await expect(interval).toHaveValue("30");
});

test("480px 低高度视口中任务末项和历史末项可达且保存入口保持可见", async ({ page, viewport }) => {
  await page.setViewportSize({ width: viewport!.width, height: 480 });
  await useInspection(page, busyInspection);
  await page.goto("/auto-inspection");
  const queue = page.getByTestId("auto-inspection-queue-scroll-area");
  const finalOperation = queue.getByText("巡检操作 18", { exact: true });
  await finalOperation.scrollIntoViewIfNeeded();
  await expect(finalOperation).toBeInViewport({ ratio: 1 });
  const table = page.getByRole("table", { name: "巡检心跳记录", exact: true });
  const finalHeartbeat = table.getByRole("button", { name: "查看心跳详情", exact: true }).last();
  await finalHeartbeat.scrollIntoViewIfNeeded();
  await expect(finalHeartbeat).toBeInViewport({ ratio: 1 });
  await finalHeartbeat.click();
  const dialog = page.getByRole("dialog", { name: "巡检心跳详情" });
  await expect(dialog).toContainText("第 30 轮探测失败");
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.keyboard.press("Escape");
  await expect(finalHeartbeat).toBeFocused();
  await expect(page.getByRole("button", { name: "保存自动巡检", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await expect(page.getByRole("heading", { name: "自动巡检", exact: true })).toBeInViewport({
    ratio: 1,
  });
});
