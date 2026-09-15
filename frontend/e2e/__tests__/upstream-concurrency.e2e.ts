import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

test("上游并发在窄屏表格内可读取且刷新后清除超限状态并保留选择", async ({ page }) => {
  let refreshed = false;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "并发回归测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/upstreams": {
        hosts: [
          {
            upstream_id: "capacity-fixture",
            host: "capacity.example.test",
            hosts: ["capacity.example.test"],
            name: "共享并发上游",
            base_url: "https://capacity.example.test",
            upstream_type: "sub2api",
            auth_status: "已鉴权",
            account_count: 2,
            group_count: 1,
            raw_balance: "10",
            balance: "10",
            recharge_rate: "1",
            balance_status: "已读取",
            checked_at: null,
            concurrency_limit: refreshed ? 20 : 10,
            concurrency_status: refreshed ? "known" : "stale",
            allocated_concurrency: 12,
            target_concurrency: refreshed ? 18 : 10,
          },
        ],
        total_hosts: 1,
        authenticated_hosts: 1,
        recovery_required: 0,
        source: "test",
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/upstreams");
  const selection = page.getByRole("checkbox", { name: "选择上游 共享并发上游" });
  await selection.check();
  const capacity = page.getByRole("group", { name: "上游并发", exact: true });
  await capacity.scrollIntoViewIfNeeded();
  await expect(capacity.getByLabel("用户上限", { exact: true })).toHaveText("10");
  await expect(capacity.getByText("已超上限", { exact: true })).toBeVisible();
  await expect(capacity.getByText("缓存额度", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );

  refreshed = true;
  await page.getByRole("button", { name: "刷新上游列表" }).click();
  await expect(capacity.getByLabel("用户上限", { exact: true })).toHaveText("20");
  await expect(capacity.getByLabel("目标并发", { exact: true })).toHaveText("18");
  await expect(capacity.getByText("已超上限", { exact: true })).toHaveCount(0);
  await expect(capacity.getByText("缓存额度", { exact: true })).toHaveCount(0);
  await expect(selection).toBeChecked();
});
