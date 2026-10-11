import { expect, test } from "@playwright/test";

import type { GroupStatus } from "../../src/api";
import { policy } from "../../src/features/accounts/__tests__/fixtures";
import { allocationFixture } from "../../src/features/groups/components/__tests__/allocation-fixture";
import { pageFixtures } from "./fixtures/page-shell";

for (const narrow of [false, true]) {
  test(`大分组调度弹窗分页保持可见，支持键盘翻页及切换每页行数${narrow ? "，320px" : ""}`, async ({
    page,
  }, testInfo) => {
    if (narrow) await page.setViewportSize({ width: 320, height: 664 });
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme),
      testInfo.project.use.colorScheme ?? "light",
    );
    const group: GroupStatus = {
      id: "6",
      name: "分页测试分组",
      account_count: 500,
      scheduling_open: 500,
      scheduling_closed: 0,
      scheduling_unknown: 0,
      strategy: "balanced",
      strategy_source: "global_default",
      participation_status: "participating",
      participation_reason: null,
      status: "healthy",
      override: null,
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "分页测试" },
        "/api/groups": [group],
        "/api/policy": policy,
        "/api/dictionaries": { items: [] },
        "/api/groups/6/allocation": allocationFixture(500),
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
    await page.getByRole("button", { name: "查看分组账号调度状态" }).click();
    const dialog = page.getByRole("dialog");
    const table = dialog.getByRole("table");
    const pagination = dialog.getByRole("navigation", { name: "表格分页" });
    await expect(table.getByRole("row")).toHaveCount(21);
    await page.screenshot({ path: testInfo.outputPath("allocation-initial.png") });
    await expect(dialog.getByRole("heading", { name: "分页测试分组 调度状态" })).toBeFocused();
    expect(await dialog.evaluate((element) => element.scrollTop)).toBe(0);
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await expect(pagination).toBeInViewport({ ratio: 1 });
    const scroll = dialog.locator('[data-slot="table-container"]');
    // Keep the header and at least two complete account rows readable on a narrow viewport.
    const scrollBounds = await scroll.boundingBox();
    const secondRowBounds = await table.getByRole("row").nth(2).boundingBox();
    expect(scrollBounds).not.toBeNull();
    expect(secondRowBounds).not.toBeNull();
    expect(secondRowBounds!.y + secondRowBounds!.height).toBeLessThanOrEqual(
      scrollBounds!.y + scrollBounds!.height,
    );
    expect(await scroll.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
      true,
    );
    await scroll.evaluate((element) => {
      element.scrollTop = 300;
    });
    const next = pagination.getByRole("button", { name: "转到下一页" });
    await next.focus();
    await next.press("Enter");
    await expect(table.getByText("账号 6-21", { exact: true })).toBeVisible();
    expect(await scroll.evaluate((element) => element.scrollTop)).toBe(0);
    await expect(next).toBeFocused();

    const pageSize = pagination.getByRole("combobox", { name: "每页行数" });
    await pageSize.focus();
    await pageSize.press("Enter");
    await expect(pageSize).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("option", { name: "10", exact: true })).toBeVisible();
    await page.keyboard.press("Home");
    await page.keyboard.press("Enter");
    await expect(pageSize).toHaveAttribute("aria-expanded", "false");
    await expect(pageSize).toBeFocused();
    await expect(table.getByRole("row")).toHaveCount(11);
    await expect(table.getByText("账号 6-1", { exact: true })).toBeVisible();
    await expect(dialog.getByText("500 / 500", { exact: true })).toBeVisible();
    await expect(pagination).toBeInViewport({ ratio: 1 });
    await page.screenshot({ path: testInfo.outputPath("allocation-pagination.png") });
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await expect(page.getByRole("button", { name: "查看分组账号调度状态" })).toBeFocused();
  });
}
