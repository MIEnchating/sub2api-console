import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("统计变化按上游聚合，长名称和展开明细在桌面及手机可查看", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const upstreamName = "测试上游_" + "long-upstream-name".repeat(8);
  const groupName = "新增分组_" + "long-group-name".repeat(12);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "聚合测试" },
      "/api/upstreams": {
        hosts: [
          {
            upstream_id: "one",
            name: upstreamName,
            host: "one.example.test",
            hosts: ["one.example.test"],
            base_url: "https://one.example.test",
            upstream_type: "sub2api",
            account_count: 0,
            group_count: 1,
            auth_status: "已鉴权",
            balance: "10",
            raw_balance: "10",
            recharge_rate: "1",
            balance_status: "已读取",
          },
        ],
        total_hosts: 1,
        authenticated_hosts: 1,
        recovery_required: 0,
        source: "test",
      },
      "/api/upstreams/group-history": [
        {
          id: 3,
          upstream_id: "one",
          group_id: "3",
          group_name: groupName,
          effective_rate: "0.1234567890123456789",
          change_type: "added",
          changed_at: "2026-09-16T03:00:00Z",
        },
        {
          id: 2,
          upstream_id: "deleted-upstream",
          group_id: "2",
          group_name: "其他上游分组",
          change_type: "added",
          changed_at: "2026-09-16T02:00:00Z",
        },
        {
          id: 1,
          upstream_id: "one",
          group_id: "1",
          group_name: "已删除分组",
          effective_rate: "9.99",
          change_type: "removed",
          changed_at: "2026-09-16T01:00:00Z",
        },
      ],
    };
    if (path.endsWith("/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    if (path in fixtures) return route.fulfill({ json: fixtures[path] });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/upstreams");
  await page.getByRole("button", { name: "上游维护" }).click();
  await page.getByRole("menuitem", { name: "统计变化" }).click();
  const dialog = page.getByRole("dialog", { name: "上游分组变化" });
  const table = dialog.getByRole("table", { name: "按上游汇总分组变化" });
  await expect(table.getByRole("row")).toHaveCount(3);
  const expand = table.getByRole("button", { name: `展开 ${upstreamName} 的变化明细` });
  await table.getByText(upstreamName, { exact: true }).click();
  const collapse = table.getByRole("button", { name: `收起 ${upstreamName} 的变化明细` });
  await expect(collapse).toHaveAttribute("aria-expanded", "true");
  const details = dialog.getByRole("region", { name: `${upstreamName} 的变化明细` });
  await expect(details.getByRole("listitem")).toHaveCount(2);
  await expect(details.getByText(groupName, { exact: true })).toBeVisible();
  await expect(details.getByText("已删除分组", { exact: true })).toBeVisible();
  await expect(
    details.getByText("账号成本（已换算）：0.1234567890123456789", { exact: true }),
  ).toBeVisible();
  const removed = details.getByRole("listitem").filter({ hasText: "已删除分组" });
  await expect(removed).not.toContainText("账号成本");
  await expect(removed).not.toContainText("9.99");
  await expect(details.getByText("其他上游分组", { exact: true })).toHaveCount(0);
  await details.getByText("已删除分组", { exact: true }).click();
  await expect(collapse).toHaveAttribute("aria-expanded", "true");
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await details.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({
    path: test.info().outputPath("upstream-history-expanded.png"),
    animations: "disabled",
  });
  await collapse.focus();
  await page.keyboard.press("Space");
  await expect(details).toHaveCount(0);
  await expect(table.getByRole("row")).toHaveCount(3);
  await expand.click();
  await expect(details).toHaveCount(1);
  await collapse.click();
  await expect(details).toHaveCount(0);
});
