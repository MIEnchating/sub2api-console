import { expect, test } from "@playwright/test";

import type { UnifiedLogEntry, UnifiedLogPage } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

const longTitle = "华东生产环境账号与分组自动巡检 / 长任务名称用于验证记录列不会挤压状态和操作";
const longSummary =
  "正在读取真实请求记录并执行主动探测，核对账号、分组和模型的可用状态；需要完整保留异常原因与后续处理建议。".repeat(
    4,
  );
const longObject = "华东生产环境专用模型分组 / 包含完整业务说明的超长对象名称";

function logEntry(index: number): UnifiedLogEntry {
  const kind = (["task", "event", "change"] as const)[index % 3];
  const source = { task: "run_record", event: "runtime_event", change: "operation_audit" } as const;
  return {
    id: `layout-log-${index + 1}`,
    kind,
    occurred_at: "2026-09-15T08:59:33Z",
    title: index === 0 ? longTitle : `巡检记录 ${index + 1}`,
    summary: index === 0 ? longSummary : "巡检完成，已核对账号与分组状态",
    status: ["running", "succeeded", "partial", "error"][index % 4],
    actor: index === 0 ? "自动巡检调度器" : null,
    object_label: index === 0 ? longObject : null,
    source: source[kind],
    source_id: String(index + 1),
    related_count: index === 0 ? 131 : 0,
    details: {},
  };
}

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "日志布局测试" },
      "/api/accounts": [],
      "/api/groups": [{ id: "layout-group", name: longObject }],
      "/api/dictionaries": { items: [] },
      "/api/tasks": [],
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (url.pathname === "/api/logs") {
      const currentPage = Number(url.searchParams.get("page") ?? 1);
      const pageSize = Number(url.searchParams.get("page_size") ?? 20);
      const kind = url.searchParams.get("kind") ?? "all";
      const entries = Array.from({ length: 24 }, (_, index) => logEntry(index)).filter(
        (entry) => kind === "all" || kind === entry.kind,
      );
      const response: UnifiedLogPage = {
        items: entries.slice((currentPage - 1) * pageSize, currentPage * pageSize),
        total: entries.length,
        page: currentPage,
        page_size: pageSize,
        counts: { task: 8, event: 8, change: 8 },
        truncated: true,
      };
      await route.fulfill({ json: response });
    } else if (url.pathname.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (route.request().method() === "GET" && url.pathname in fixtures) {
      await route.fulfill({ json: fixtures[url.pathname] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("长摘要使用 12px 辅助字号，标题保持 14px 并限制摘要为两行", async ({ page }) => {
  await page.goto("/logs");
  const table = page.getByRole("table");
  const summary = table.getByText(longSummary, { exact: false });
  await expect(summary).toHaveCSS("font-size", "12px");
  await expect(table.getByText(longTitle, { exact: true })).toHaveCSS("font-size", "14px");
  await expect(summary).toHaveCSS("-webkit-line-clamp", "2");
  await expect(summary).toHaveCSS("overflow", "hidden");
  const record = table.getByRole("cell").filter({ hasText: longSummary });
  await expect(record.getByText("关联 131 条", { exact: true })).toBeVisible();
  expect(await summary.getByText("关联 131 条", { exact: true }).count()).toBe(0);
});

test("长标题和对象限制在固定列内，横向溢出留在表格容器", async ({ page, viewport }) => {
  await page.goto("/logs");
  const table = page.getByRole("table", { name: "日志记录" });
  await expect(table.getByRole("columnheader")).toHaveCount(6);
  await expect(table).toHaveCSS("table-layout", "fixed");
  const title = table.getByText(longTitle, { exact: true });
  const object = table.getByText(longObject, { exact: true });
  await expect(title).toHaveCSS("text-overflow", "ellipsis");
  await expect(object).toHaveCSS("text-overflow", "ellipsis");
  expect(await title.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
  expect(await object.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
  await expect(table.getByRole("columnheader", { name: "对象 / 执行人" })).toHaveCSS(
    "width",
    viewport!.width >= 1280 ? "208px" : "176px",
  );
  await expect(table.getByRole("columnheader", { name: "操作", exact: true })).toHaveCSS(
    "width",
    "64px",
  );
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("logs-center.png"), fullPage: true });
});

test("日志表格独立滚动并固定表头，分页留在表格外且可翻页", async ({ page }) => {
  await page.goto("/logs");
  const table = page.getByRole("table", { name: "日志记录" });
  await expect(table.getByText(longTitle, { exact: true })).toBeVisible();
  const container = page.locator('[data-slot="table-container"]');
  const pagination = page.getByRole("navigation", { name: "表格分页" });
  await expect(container).toHaveCSS("overflow-y", "auto");
  await expect(container).toHaveCSS("overflow-x", "auto");
  expect(await container.getByRole("navigation", { name: "表格分页" }).count()).toBe(0);
  await container.scrollIntoViewIfNeeded();
  await container.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect.poll(() => container.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await expect(table.getByRole("columnheader", { name: "时间", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await pagination.scrollIntoViewIfNeeded();
  const next = pagination.getByRole("button", { name: "转到下一页", exact: true });
  await expect(next).toBeInViewport({ ratio: 1 });
  await next.click();
  await expect(table.getByText("巡检记录 21", { exact: true })).toBeVisible();
  await expect(table.locator("tbody tr")).toHaveCount(4);
});

test("320px 窄屏完整展示四个类型入口，搜索与条件筛选不撑宽页面", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 720 });
  await page.goto("/logs");
  const tabs = page.getByRole("tablist", { name: "记录类型" });
  for (const label of ["全部记录", "任务记录", "事件日志", "远程读写"]) {
    await expect(tabs.getByRole("tab", { name: label, exact: true })).toBeInViewport({ ratio: 1 });
  }
  expect(await tabs.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(page.getByRole("textbox", { name: "搜索任务、对象或原因" })).toBeInViewport({
    ratio: 1,
  });
  await expect(page.getByRole("button", { name: "执行结果筛选" })).toBeInViewport({ ratio: 1 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("logs-center-narrow.png"), fullPage: true });
});

test("键盘切换记录类型后同步选中状态，并能打开和关闭日志详情", async ({ page }) => {
  await page.goto("/logs");
  const tabs = page.getByRole("tablist", { name: "记录类型" });
  const all = tabs.getByRole("tab", { name: "全部记录", exact: true });
  await all.focus();
  await all.press("ArrowRight");
  const task = tabs.getByRole("tab", { name: "任务记录", exact: true });
  await expect(task).toBeFocused();
  await expect(task).toHaveAttribute("aria-selected", "true");
  await expect(page).toHaveURL(/kind=task/);
  const table = page.getByRole("table", { name: "日志记录" });
  const details = table.getByRole("button", { name: /查看.*详情/ }).first();
  await details.focus();
  await details.press("Enter");
  const dialog = page.getByRole("dialog", { name: longTitle });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText(longSummary, { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await expect(details).toBeFocused();
});

test("日志为空时说明留在可见视口内，无需横向滚动表格", async ({ page }) => {
  await page.route("**/api/logs?**", (route) =>
    route.fulfill({
      json: { items: [], total: 0, page: 1, page_size: 20, counts: {}, truncated: false },
    }),
  );
  await page.goto("/logs");

  await expect(page.getByText("暂无日志记录", { exact: true })).toBeInViewport({ ratio: 1 });
  await expect(page.getByText("任务执行和系统事件将显示在这里", { exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(
    await page.locator('[data-slot="table-container"]').evaluate((element) => element.scrollLeft),
  ).toBe(0);
});
