import { expect, test } from "@playwright/test";

import type { GroupStatus } from "../../src/api";
import { policy } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

const groups: GroupStatus[] = Array.from({ length: 19 }, (_, index) => ({
  id: String(index + 1),
  name: `测试分组 ${index + 1}`,
  account_count: 1,
  scheduling_open: 1,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "balanced",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "healthy",
  override: null,
}));

test("勾选和清空分组时表格高度与分页位置保持不变，批量操作条浮动显示", async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/groups": groups,
      "/api/policy": policy,
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/groups");
  const selectPage = page.getByRole("checkbox", { name: "选择当前页分组", exact: true });
  await expect(selectPage).toBeVisible();
  const panel = page.locator("[data-table-panel]");
  const pagination = page.getByRole("button", { name: "转到下一页" });
  const originalHeight = await panel.evaluate((element) => element.clientHeight);
  const originalPaginationOffset = await pagination.evaluate(
    (element) => (element as HTMLButtonElement).offsetTop,
  );

  await selectPage.click();
  const toolbar = page.getByRole("toolbar", { name: "已选择 19 个分组的批量操作" });
  await expect(toolbar).toBeInViewport({ ratio: 1 });
  await expect(toolbar).toHaveCSS("position", "fixed");
  expect(await panel.evaluate((element) => element.clientHeight)).toBe(originalHeight);
  expect(await pagination.evaluate((element) => (element as HTMLButtonElement).offsetTop)).toBe(
    originalPaginationOffset,
  );

  await toolbar.getByRole("button", { name: "清空选择" }).click();
  await expect(toolbar).toHaveCount(0);
  await expect(selectPage).toHaveAttribute("aria-checked", "false");
  expect(await panel.evaluate((element) => element.clientHeight)).toBe(originalHeight);
});
