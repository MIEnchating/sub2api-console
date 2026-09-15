import { expect, test, type Page } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { setupKuma } from "../kuma-fixture";

const cases = [
  { path: "/accounts", endpoint: "/api/accounts", empty: "当前没有账号", data: [] },
  { path: "/groups", endpoint: "/api/groups", empty: "当前没有分组", data: [] },
  { path: "/system-info", endpoint: "/api/tasks", empty: "当前没有进行中的任务", data: [] },
  {
    path: "/traffic",
    endpoint: "/api/traffic/ranking",
    empty: "当前范围没有匹配的账号流量",
    data: { accounts: [] },
  },
  {
    path: "/vault",
    endpoint: "/api/auth-recovery/config",
    empty: "暂无凭据",
    data: { vault_entries: [], auth_records: [] },
  },
  {
    path: "/logs?kind=event",
    endpoint: "/api/logs",
    empty: "暂无日志记录",
    data: { items: [], total: 0, page: 1, page_size: 20, counts: {}, truncated: false },
  },
];

async function mockPage(
  page: Page,
  endpoint: string,
  data: unknown,
  theme: string | null,
): Promise<() => void> {
  let fail = true;
  await page.addInitScript(
    (value) => localStorage.setItem("sub2api-console-theme", value ?? "light"),
    theme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === endpoint) {
      await route.fulfill({
        status: fail ? 503 : 200,
        json: fail ? { detail: "数据读取暂不可用" } : data,
      });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "状态复查" },
      "/api/groups": [],
      "/api/accounts": [],
      "/api/tasks": [],
      "/api/dictionaries": { items: [] },
      "/api/system/metrics": {
        cpu: { usage_percent: 20, logical_cores: 4 },
        memory: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
        disk: { usage_percent: 40, used_bytes: 1024, total_bytes: 4096 },
      },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  return () => {
    fail = false;
  };
}

test("日志后台刷新失败后保留已读取记录和分页", async ({ page, colorScheme }) => {
  await mockPage(page, "/unused", [], colorScheme);
  let fail = false;
  await page.route("**/api/logs?*", (route) =>
    route.fulfill({
      status: fail ? 503 : 200,
      json: fail
        ? { detail: "日志刷新暂不可用" }
        : {
            items: [
              {
                id: "event-1",
                kind: "event",
                occurred_at: "2026-09-01T00:00:00Z",
                title: "巡检完成",
                summary: "已检查所有账号",
                status: "succeeded",
                actor: null,
                object_label: null,
                source: "event",
                source_id: "event-1",
                related_count: 0,
                details: {},
              },
            ],
            total: 21,
            page: 1,
            page_size: 20,
            counts: {},
            truncated: false,
          },
    }),
  );
  await page.goto("/logs?kind=event");
  await expect(page.getByRole("cell").filter({ hasText: "巡检完成" })).toBeVisible();
  fail = true;
  await page.getByRole("button", { name: "刷新日志" }).click();
  await expect(
    page.locator("[data-sonner-toast]").filter({ hasText: "日志刷新暂不可用" }),
  ).toBeVisible({ timeout: 15000 });
  await expect(page.getByRole("cell").filter({ hasText: "巡检完成" })).toBeVisible();
  await expect(page.getByRole("button", { name: "转到下一页" })).toBeVisible();
});

test("系统设置后台刷新失败保留表单草稿和分类导航", async ({ page, colorScheme }) => {
  await mockPage(page, "/unused", [], colorScheme);
  let fail = false;
  await page.route("**/api/config", (route) =>
    route.fulfill({
      status: fail ? 503 : 200,
      json: fail ? { detail: "配置刷新暂不可用" } : pageFixtures["/api/config"],
    }),
  );
  await page.goto("/config");
  const url = page.getByRole("textbox", { name: "Sub2API 地址", exact: true });
  await url.fill("https://draft.example.test");
  fail = true;
  await page.getByRole("button", { name: "刷新系统设置" }).click();
  await expect(
    page.locator("[data-sonner-toast]").filter({ hasText: "配置刷新暂不可用" }),
  ).toBeVisible({ timeout: 15000 });
  await expect(url).toHaveValue("https://draft.example.test");
  await expect(page.getByRole("navigation", { name: "系统设置分类导航" })).toBeVisible();
});

test("密码箱后台刷新失败保留索引并禁用修改入口", async ({ page, colorScheme }) => {
  await mockPage(page, "/unused", [], colorScheme);
  let fail = false;
  await page.route("**/api/auth-recovery/config", (route) =>
    route.fulfill({
      status: fail ? 503 : 200,
      json: fail
        ? { detail: "索引刷新暂不可用" }
        : {
            vault_entries: [
              {
                entry: "运营账号",
                hosts: ["upstream.example.test"],
                header_names: [],
                has_username: true,
                has_password: true,
                username_is_email: true,
              },
            ],
            auth_records: [],
          },
    }),
  );
  await page.goto("/vault");
  await expect(page.getByRole("cell", { name: "运营账号", exact: true })).toBeVisible();
  fail = true;
  await page.getByRole("button", { name: "刷新密码箱" }).click();
  await expect(
    page.locator("[data-sonner-toast]").filter({ hasText: "索引刷新暂不可用" }),
  ).toBeVisible({ timeout: 15000 });
  await expect(page.getByRole("cell", { name: "运营账号", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "编辑凭据", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "删除凭据", exact: true })).toBeDisabled();
});

for (const resource of ["templates", "resources/status-pages"]) {
  test(`监控 ${resource} 读取失败保留重试入口且不显示空数据`, async ({ page }) => {
    await setupKuma(page);
    let fail = true;
    await page.route(`**/api/uptime-kuma/${resource}`, (route) =>
      fail ? route.fulfill({ status: 404, json: { detail: "读取暂不可用" } }) : route.fallback(),
    );
    await page.goto(`/uptime-kuma/${resource.replace("resources/", "")}`);
    const retry = page.getByRole("button", { name: "重新读取", exact: true });
    await expect(retry).toBeVisible({ timeout: 15000 });
    await expect(page.getByText(/暂无记录|暂无功能模板/)).toHaveCount(0);
    await expect(retry).toBeInViewport({ ratio: 1 });
    fail = false;
    await retry.click();
    await expect(
      page.getByRole("cell", {
        name: resource === "templates" ? "API JSON 模板" : "服务状态",
        exact: true,
      }),
    ).toBeVisible();
    await expect(retry).toHaveCount(0);
  });
}

for (const scenario of cases) {
  test(`${scenario.path} 首次读取失败不冒充空数据，窄表格重试可见并恢复空结果`, async ({
    page,
    colorScheme,
  }) => {
    const succeed = await mockPage(page, scenario.endpoint, scenario.data, colorScheme);
    await page.goto(scenario.path);
    const retry = page.getByRole("button", { name: "重新读取", exact: true });
    await expect(retry).toBeVisible({ timeout: 15000 });
    await expect(page.getByText(scenario.empty, { exact: true })).toHaveCount(0);
    await retry.scrollIntoViewIfNeeded();
    await expect(retry).toBeInViewport({ ratio: 1 });
    await page.screenshot({ path: test.info().outputPath("query-retry.png") });
    succeed();
    await retry.click();
    await expect(retry).toHaveCount(0);
    const empty = page.getByText(scenario.empty, { exact: true });
    await expect(empty).toBeVisible();
    await empty.scrollIntoViewIfNeeded();
    await expect(empty).toBeInViewport({ ratio: 1 });
  });
}
