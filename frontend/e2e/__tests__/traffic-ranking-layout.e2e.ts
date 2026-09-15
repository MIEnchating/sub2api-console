import { expect, test } from "@playwright/test";

import type { TrafficRanking, TrafficRankingRow } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

const longAccountName =
  "华东生产主账号 / 长账号名称用于验证排行中完整身份可读取且不会挤压相邻流量数据";
const longGroupName = "华东生产环境专用模型分组 / 包含完整业务说明的长分组名称";
const upstreamHost = "traffic-layout.example.test";

function trafficAccount(index: number): TrafficRankingRow {
  return {
    rank: index + 1,
    account_id: String(index + 41),
    account_name: index === 0 ? longAccountName : `备用账号 ${index + 1}`,
    upstream_host: upstreamHost,
    platform: "anthropic",
    groups: [longGroupName],
    requests: 1250,
    successful: 1245,
    failed: 5,
    traffic_share: 4.17,
    success_rate: 99.6,
    stability_score: 99.08,
    average_latency_ms: 820,
    p95_latency_ms: 1450,
    active_buckets: 22,
    total_buckets: 24,
    latest_at: "2026-09-15T11:58:00Z",
    input_tokens: 120000,
    output_tokens: 45000,
    cache_read_tokens: 8000,
    cache_write_tokens: 1200,
    usage_available: true,
  };
}

const ranking: TrafficRanking = {
  start_at: "2026-09-14T12:00:00Z",
  end_at: "2026-09-15T12:00:00Z",
  group_name: "",
  sort_by: "traffic",
  bucket: "hour",
  total_requests: 30000,
  accounts_with_traffic: 24,
  accounts: Array.from({ length: 24 }, (_, index) => trafficAccount(index)),
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
      "/api/auth/session": { authenticated: true, username: "流量布局测试" },
      "/api/accounts": [],
      "/api/groups": [{ id: "traffic-group", name: longGroupName }],
      "/api/dictionaries": { items: [] },
      "/api/tasks": [],
      "/api/traffic/ranking": ranking,
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("长账号名称限制在固定身份列内，并保留完整名称和账号元信息", async ({ page, viewport }) => {
  await page.goto("/traffic");
  const table = page.getByRole("table", { name: "账号流量排行" });
  await expect(table.getByRole("columnheader")).toHaveCount(8);
  const heading = table.getByRole("columnheader", { name: "排名 / 账号", exact: true });
  await expect(heading).toHaveCSS("width", viewport!.width >= 640 ? "256px" : "192px");
  const name = table.getByText(longAccountName, { exact: true });
  await expect(name).toHaveCSS("text-overflow", "ellipsis");
  expect(await name.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(true);
  await expect(name).not.toHaveAttribute("title");
  await name.hover();
  await expect(page.locator('[data-slot="tooltip-content"]')).toHaveText(longAccountName);
  await page.keyboard.press("Escape");
  await expect(name).toHaveText(longAccountName);
  const metadata = table.getByText(`#41 · ${longGroupName}`, { exact: true });
  await expect(metadata).toHaveCSS("font-size", "12px");
  await expect(metadata).not.toHaveAttribute("title");
  await metadata.hover();
  await expect(page.locator('[data-slot="tooltip-content"]')).toHaveText(`#41 · ${longGroupName}`);
  await page.keyboard.press("Escape");
  await expect(table.getByText(upstreamHost, { exact: true }).first()).toHaveCSS(
    "font-size",
    "12px",
  );
});

test("浅色与深色背景上的前三名和当前排序文字达到正文对比度", async ({ page }) => {
  await page.goto("/traffic");
  const table = page.getByRole("table", { name: "账号流量排行" });
  await expect(table).toBeVisible();
  const contrasts = await table
    .locator('[aria-label="第 1 名"], [aria-sort] > div:first-child')
    .evaluateAll((elements) => {
      const context = document.createElement("canvas").getContext("2d")!;
      function luminance(): number {
        return Array.from(context.getImageData(0, 0, 1, 1).data)
          .slice(0, 3)
          .map((value) => value / 255)
          .map((value) => (value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4))
          .reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
      }
      return elements.map((element) => {
        const ancestors: Element[] = [];
        for (let ancestor: Element | null = element; ancestor; ancestor = ancestor.parentElement) {
          ancestors.unshift(ancestor);
        }
        context.fillStyle = "white";
        context.fillRect(0, 0, 1, 1);
        for (const ancestor of ancestors) {
          context.fillStyle = getComputedStyle(ancestor).backgroundColor;
          context.fillRect(0, 0, 1, 1);
        }
        const background = luminance();
        context.fillStyle = getComputedStyle(element).color;
        context.fillRect(0, 0, 1, 1);
        const foreground = luminance();
        return (
          (Math.max(background, foreground) + 0.05) / (Math.min(background, foreground) + 0.05)
        );
      });
    });
  expect(contrasts).toHaveLength(2);
  for (const contrast of contrasts) expect(contrast).toBeGreaterThanOrEqual(4.5);
});

test("表格双向滚动时固定账号与表头，分页可独立到达并翻页", async ({ page }) => {
  await page.goto("/traffic");
  const table = page.getByRole("table", { name: "账号流量排行" });
  await expect(table.getByText(longAccountName, { exact: true })).toBeVisible();
  const container = page.locator('[data-slot="table-container"]');
  const pagination = page.getByRole("navigation", { name: "表格分页" });
  await container.scrollIntoViewIfNeeded();
  expect(await container.getByRole("navigation", { name: "表格分页" }).count()).toBe(0);
  await expect(container).toHaveCSS("overflow-x", "auto");
  await expect(container).toHaveCSS("overflow-y", "auto");
  const firstCell = table.locator("tbody tr").first().getByRole("cell").first();
  await expect(firstCell).toHaveCSS("position", "sticky");
  await expect(firstCell).toHaveCSS("left", "0px");
  await container.evaluate((element) => element.scrollTo(element.scrollWidth, 0));
  await expect.poll(() => container.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  await expect(table.getByText(longAccountName, { exact: true })).toBeInViewport({
    ratio: 1,
  });
  await container.evaluate((element) =>
    element.scrollTo(element.scrollWidth, element.scrollHeight),
  );
  await expect.poll(() => container.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await expect(
    table.getByRole("columnheader", { name: "排名 / 账号", exact: true }),
  ).toBeInViewport({
    ratio: 1,
  });
  await expect(table.getByText("备用账号 20", { exact: true })).toBeInViewport({
    ratio: 1,
  });
  await pagination.scrollIntoViewIfNeeded();
  const next = pagination.getByRole("button", { name: "转到下一页", exact: true });
  await expect(next).toBeInViewport({ ratio: 1 });
  await next.click();
  await expect(table.getByText("备用账号 21", { exact: true })).toBeVisible();
  await expect(table.locator("tbody tr")).toHaveCount(4);
});

test("键盘选择长分组并关闭筛选器后恢复焦点，页面不产生横向滚动", async ({ page }) => {
  await page.goto("/traffic");
  const table = page.getByRole("table", { name: "账号流量排行" });
  await expect(table).toBeVisible();
  const groupFilter = page.getByRole("button", { name: "账号分组筛选", exact: true });
  await groupFilter.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("combobox", { name: "搜索账号分组", exact: true })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("option", { name: longGroupName, exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.keyboard.press("Escape");
  await expect(groupFilter).toBeFocused();
  await expect(groupFilter).toHaveAttribute("aria-expanded", "false");
  for (const label of ["时间范围筛选", "账号分组筛选", "平台筛选", "排行维度筛选"]) {
    await expect(page.getByRole("button", { name: label, exact: true })).toBeInViewport({
      ratio: 1,
    });
  }
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await expect(page.getByRole("heading", { name: "流量排行", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.screenshot({ path: test.info().outputPath("traffic-ranking.png") });
});

test("刷新请求尚未完成时保留已有排行和分页且不显示首次加载骨架", async ({ page }) => {
  await page.goto("/traffic");
  const table = page.getByRole("table", { name: "账号流量排行" });
  const account = table.getByText(longAccountName, { exact: true });
  await expect(account).toBeVisible();
  let releaseResponse = (): void => {};
  const responseGate = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });
  await page.route("**/api/traffic/ranking?*", async (route) => {
    await responseGate;
    await route.fulfill({ json: ranking });
  });
  const refresh = page.getByRole("button", { name: "刷新流量排行", exact: true });
  try {
    await refresh.click();
    await expect(refresh).toBeDisabled();
    await expect(account).toBeVisible();
    await expect(page.getByRole("navigation", { name: "表格分页" })).toBeVisible();
    await expect(page.getByRole("status", { name: "流量排行加载中" })).toHaveCount(0);
  } finally {
    releaseResponse();
  }
  await expect(refresh).toBeEnabled();
});
