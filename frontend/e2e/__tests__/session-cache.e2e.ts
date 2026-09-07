import { expect, test } from "@playwright/test";

test("退出后重新登录时读取新日志，不复用上次会话的业务缓存", async ({ page }) => {
  let authenticated = true;
  let summary = "退出前的运行记录";
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/auth/logout") authenticated = false;
    if (path === "/api/auth/login") {
      authenticated = true;
      summary = "重新登录后的运行记录";
    }
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated, username: authenticated ? "operator" : null },
      "/api/auth/login": { authenticated: true, username: "operator" },
      "/api/auth/logout": { authenticated: false, username: null },
      "/api/overview": { mode: "监控模式", account_count: 1, group_count: 0, open_alerts: 0 },
      "/api/groups": [],
      "/api/logs": {
        items: [
          {
            id: "event:1",
            kind: "event",
            occurred_at: "2026-09-07T00:00:00Z",
            title: "运行事件",
            summary,
            status: "succeeded",
            actor: null,
            object_label: null,
            source: "runtime_event",
            source_id: "1",
            related_count: 0,
            details: {},
          },
        ],
        total: 1,
        page: 1,
        page_size: 20,
        counts: {},
        truncated: false,
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
      return;
    }
    if (!(path in responses)) {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      return;
    }
    await route.fulfill({ json: responses[path] });
  });

  await page.goto("/logs?kind=event");
  await expect(page.getByText("退出前的运行记录", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("heading", { name: "登录", exact: true })).toBeVisible();
  await page.getByLabel("账号", { exact: true }).fill("operator");
  await page.getByLabel("密码", { exact: true }).fill("test-password");
  await page.getByRole("button", { name: "登录", exact: true }).click();

  await expect(page.getByText("重新登录后的运行记录", { exact: true })).toBeVisible();
  await expect(page.getByText("退出前的运行记录", { exact: true })).toHaveCount(0);
});

test("退出请求失败时保留当前会话并显示可重试的失败提示", async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/auth/logout") {
      await route.fulfill({ status: 503, json: { detail: "退出失败，请重试" } });
      return;
    }
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "operator" },
      "/api/overview": { mode: "监控模式", account_count: 0, group_count: 0, open_alerts: 0 },
      "/api/groups": [],
      "/api/logs": { items: [], total: 0, page: 1, page_size: 20, counts: {}, truncated: false },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
      return;
    }
    if (!(path in responses)) {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      return;
    }
    await route.fulfill({ json: responses[path] });
  });
  await page.goto("/logs?kind=event");
  await expect(page.getByText("暂无日志记录", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "退出登录" }).click();

  await expect(page.getByText("退出失败，请重试", { exact: true })).toBeVisible();
  await expect(page.getByText("暂无日志记录", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "登录", exact: true })).toHaveCount(0);
});
