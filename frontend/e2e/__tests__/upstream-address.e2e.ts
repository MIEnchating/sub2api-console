import { expect, test } from "@playwright/test";

for (const aliases of [false, true]) {
  test(`上游地址更新后列表链接和键盘提示同步更新${aliases ? "并保留关联域名" : ""}`, async ({
    page,
  }) => {
    let baseURL = "https://old.example.test";
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const responses: Record<string, unknown> = {
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "地址回归测试" },
        "/api/config": { mode: "完全模式", values: {} },
        "/api/inspection/automation": {
          enabled: false,
          running: false,
          traffic_collection: { enabled: false },
        },
        "/api/upstreams": {
          hosts: [
            {
              upstream_id: "up_test",
              host: "old.example.test",
              hosts: aliases ? ["old.example.test", "alias.example.test"] : ["old.example.test"],
              name: "测试上游",
              base_url: baseURL,
              account_base_url: "https://models.example.test/v1",
              upstream_type: "sub2api",
              auth_status: "authenticated",
              account_count: 0,
              group_count: 0,
              raw_balance: null,
              balance: null,
              display_balance: null,
              balance_unit: null,
              recharge_rate: "1",
              balance_status: "未读取",
              checked_at: null,
            },
          ],
          total_hosts: 1,
          authenticated_hosts: 1,
          recovery_required: 0,
          source: "test",
        },
      };
      if (path.endsWith("/events")) {
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
      } else if (route.request().method() === "GET" && path in responses) {
        await route.fulfill({ json: responses[path] });
      } else {
        await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      }
    });
    await page.goto("/upstreams");
    await expect(
      page.getByRole("link", { name: "访问 测试上游（old.example.test）" }),
    ).toBeVisible();

    baseURL = "https://new.example.test:8443/admin";
    await page.getByRole("button", { name: "刷新上游列表" }).click();

    const link = page.getByRole("link", { name: "访问 测试上游（new.example.test:8443）" });
    await expect(link).toHaveText("new.example.test:8443");
    await expect(link).toHaveAttribute("href", baseURL);
    await link.focus();
    await page.keyboard.press("Shift+Tab");
    await page.keyboard.press("Tab");
    await expect(link).toBeFocused();
    const tooltip = page.getByText(baseURL, { exact: !aliases });
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toContainText(baseURL);
    if (aliases) {
      await expect(tooltip).toContainText("alias.example.test");
    } else {
      await expect(tooltip).not.toContainText("old.example.test");
    }
  });
}
