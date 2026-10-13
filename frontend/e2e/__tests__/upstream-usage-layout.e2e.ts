import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

for (const narrow of [false, true]) {
  test(`上游列表账号明细分页且不撑高原行${narrow ? "，320px 长金额" : ""}`, async ({
    page,
  }, testInfo) => {
    if (narrow) await page.setViewportSize({ width: 320, height: 664 });
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme),
      testInfo.project.use.colorScheme ?? "light",
    );
    function spendingTotal(index: number): string {
      if (narrow) return "123456789012345678901234567890.123456";
      return index === 20 ? "20" : "0.1";
    }
    const hosts = Array.from({ length: 21 }, (_, index) => ({
      upstream_id: `upstream-${index}`,
      host: `upstream-${index}.example.test`,
      hosts: [`upstream-${index}.example.test`],
      base_url: `https://upstream-${index}.example.test`,
      name: `上游 ${index + 1}`,
      upstream_type: "newapi",
      account_count: 500,
      group_count: 5,
      auth_status: "已鉴权",
      raw_balance: "100",
      balance: "100",
      recharge_rate: "1",
      balance_status: "已读取",
      checked_at: null,
      spending: {
        total: spendingTotal(index),
        started_at: "2026-10-09",
        updated_at: "2026-10-09",
      },
      request_count: index === 20 ? 500 : 1000,
      request_window_days: 30,
      account_request_usage: Array.from({ length: 500 }, (_, account) => ({
        account_id: `${account}`,
        account_name: `账号 ${account} ${"较长的账号名称".repeat(5)}`,
        request_count: 20,
        share: 2,
      })),
    }));
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": {
          initialized: true,
          target_configured: true,
          configuration_errors: [],
        },
        "/api/auth/session": { authenticated: true, username: "测试用户" },
        "/api/dictionaries": { items: [] },
        "/api/upstreams/usage": hosts.map((host) => ({
          upstream_id: host.upstream_id,
          request_count: host.request_count,
          request_window_days: host.request_window_days,
          account_request_usage: host.account_request_usage,
        })),
        "/api/upstreams": {
          hosts,
          total_hosts: hosts.length,
          authenticated_hosts: hosts.length,
          recovery_required: 0,
          source: "test",
        },
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
      else await route.fulfill({ json: fixtures[path] ?? [] });
    });
    await page.goto("/upstreams");
    const trigger = page.getByRole("button", {
      name: "查看上游 1的账号使用率",
      exact: true,
      includeHidden: true,
    });
    await expect(trigger).toBeVisible();
    await expect(page.getByRole("region", { name: "全部上游汇总" })).toHaveCount(0);
    const row = page.getByRole("row", { includeHidden: true }).filter({ has: trigger });
    const height = await row.evaluate((element) => element.getBoundingClientRect().height);
    expect(height).toBeLessThan(150);
    // Reveal the detail control within the wide table's scrollable content,
    // accounting for its pinned action column on narrow viewports.
    await trigger.evaluate((element) => {
      const container = element.closest<HTMLElement>('[data-slot="table-container"]');
      const actions = element.closest("tr")?.lastElementChild;
      if (!container || !actions) throw new Error("上游表格结构缺失");
      const hiddenWidth =
        element.getBoundingClientRect().right - actions.getBoundingClientRect().left;
      if (hiddenWidth > 0) container.scrollLeft += hiddenWidth + 8;
    });
    expect(
      await trigger.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        return element.contains(
          document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2),
        );
      }),
    ).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("upstream-overview.png"), fullPage: true });
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: "上游 1 · 使用统计" });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "上游 1 · 使用统计" })).toBeFocused();
    const body = dialog.locator('[data-slot="dialog-body"]');
    expect(await body.evaluate((element) => element.scrollTop)).toBe(0);
    await page.keyboard.press("Tab");
    await expect(dialog.locator("dd").first().locator("span")).toBeFocused();
    await expect(
      page.locator('[data-slot="tooltip-content"]').filter({ hasText: spendingTotal(0) }),
    ).toHaveText(spendingTotal(0));
    await page.keyboard.press("Tab");
    await expect(
      dialog.getByText(hosts[0].account_request_usage[0].account_name, { exact: true }),
    ).toBeFocused();
    await expect(
      page.locator('[data-slot="tooltip-content"]').filter({
        hasText: `${hosts[0].account_request_usage[0].account_name} · ID 0`,
      }),
    ).toHaveText(`${hosts[0].account_request_usage[0].account_name} · ID 0`);
    await expect(dialog.getByRole("table", { name: "账号使用率" })).toBeVisible();
    await expect(dialog.getByRole("row")).toHaveCount(21);
    await expect(dialog.getByText("ID 20", { exact: true })).toHaveCount(0);
    expect(await row.evaluate((element) => element.getBoundingClientRect().height)).toBe(height);
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    expect(
      await dialog.locator("dl").evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath("upstream-usage-totals.png"),
      fullPage: true,
    });
    expect(await body.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
      true,
    );
    await body.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await dialog.getByRole("button", { name: "转到下一页" }).click();
    await expect(dialog.getByText("ID 20", { exact: true })).toBeVisible();
    await expect(dialog.getByText("ID 29", { exact: true })).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("upstream-account-dialog.png"),
      fullPage: true,
    });
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await expect(trigger).toBeFocused();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
}
