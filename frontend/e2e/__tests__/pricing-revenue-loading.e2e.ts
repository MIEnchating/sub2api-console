import { expect, test, type Page, type Route } from "@playwright/test";
import { pricing } from "./fixtures/settings";
import { revenueTask } from "../../src/features/pricing/components/__tests__/revenue-fixtures";

async function isolate(page: Page, endpoint: string, theme: string | null): Promise<Route[]> {
  const held: Route[] = [];
  await page.addInitScript(
    (value) => localStorage.setItem("sub2api-console-theme", value ?? "light"),
    theme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === endpoint) {
      held.push(route);
      return;
    }
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价格读取测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": { items: [] },
      "/api/pricing/backups": [],
      "/api/pricing/revenue/latest": null,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  return held;
}

test("价格目录加载前后搜索栏和表格位置不变，仅有一个读取状态", async ({ page, colorScheme }) => {
  const held = await isolate(page, "/api/pricing", colorScheme);
  await page.goto("/pricing");
  const loading = page.getByRole("status", { name: "正在读取价格目录", exact: true });
  await expect(loading).toBeVisible();
  await expect(page.getByTestId("pricing-loading").getByRole("status")).toHaveCount(0);
  const search = page.getByRole("textbox", { name: "搜索分组、ID 或平台" });
  await expect(search).toBeInViewport({ ratio: 1 });
  const before = (await loading.boundingBox())!;
  const searchBefore = (await search.boundingBox())!;
  expect(await loading.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await expect.poll(() => held.length).toBe(1);
  await held[0].fulfill({ json: pricing });
  const table = page.getByTestId("pricing-catalog-table-frame");
  await expect(table).toBeVisible();
  const after = (await table.boundingBox())!;
  expect(after.y).toBe(before.y);
  expect(after.width).toBe(before.width);
  expect((await search.boundingBox())!.y).toBe(searchBefore.y);
  await expect(loading).toHaveCount(0);
});

test("收益报告加载时预留分类导航并匹配十二列表格，读取完成不移动表格顶部", async ({
  page,
  colorScheme,
}) => {
  const held = await isolate(page, "/api/pricing/revenue/latest", colorScheme);
  await page.goto("/revenue-analysis");
  const loading = page.getByRole("status", { name: "正在读取最近一次分析" });
  await expect(loading).toBeVisible();
  await expect(loading.locator("thead th")).toHaveCount(12);
  const navigation = page.getByTestId("revenue-navigation-skeleton");
  await expect(navigation).toBeVisible();
  const before = (await loading.boundingBox())!;
  const navigationBefore = (await navigation.boundingBox())!;
  expect(
    await page
      .locator('[data-slot="page-content"]')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  await expect.poll(() => held.length).toBe(1);
  await held[0].fulfill({ json: revenueTask() });
  const tabs = page.getByRole("tablist", { name: "收益分析视图" });
  await expect(tabs).toBeVisible();
  expect((await tabs.boundingBox())!.height).toBe(navigationBefore.height);
  const panel = page.getByRole("tabpanel", { name: "账号明细" });
  expect((await panel.boundingBox())!.y).toBe(before.y);
  expect((await panel.boundingBox())!.width).toBe(before.width);
  await expect(loading).toHaveCount(0);
});

test("收益报告首次失败展示重试，成功确认无历史报告后才显示空状态", async ({
  page,
  colorScheme,
}) => {
  await isolate(page, "/unused", colorScheme);
  let failed = true;
  await page.route("**/api/pricing/revenue/latest", (route) =>
    failed
      ? route.fulfill({ status: 503, json: { detail: "报告暂时不可用" } })
      : route.fulfill({ json: null }),
  );
  await page.goto("/revenue-analysis");
  const retry = page.getByRole("button", { name: "重新读取" });
  await expect(retry).toBeVisible({ timeout: 15_000 });
  await expect(page.getByText("尚未生成核算结果")).toHaveCount(0);
  failed = false;
  await retry.click();
  await expect(page.getByText("尚未生成核算结果")).toBeVisible();
});

test("收益核算创建请求尚未返回时没有虚构进度，真实任务返回后显示百分比并允许取消", async ({
  page,
  colorScheme,
}) => {
  const held = await isolate(page, "/api/pricing/revenue", colorScheme);
  const running = {
    ...revenueTask(),
    status: "running",
    progress: 45,
    message: "正在核算",
    result: {},
  };
  await page.route("**/api/tasks/revenue-layout", (route) => route.fulfill({ json: running }));
  await page.goto("/revenue-analysis");
  await page.getByRole("button", { name: "开始分析", exact: true }).click();
  await expect(page.getByRole("status", { name: "正在创建收益核算任务" })).toBeVisible();
  await expect(page.getByRole("progressbar")).toHaveCount(0);
  await expect(page.getByText("0%", { exact: true })).toHaveCount(0);
  await expect.poll(() => held.length).toBe(1);
  await held[0].fulfill({ json: running });
  await expect(page.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "45");
  await expect(page.getByRole("button", { name: "取消任务" })).toBeEnabled();
});
