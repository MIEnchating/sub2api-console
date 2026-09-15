import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "样式复查" },
      "/api/groups": [],
      "/api/accounts": [],
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/logs": {
        items: [],
        total: 10000,
        page: 1,
        page_size: 20,
        counts: {},
        truncated: false,
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (path === "/api/dictionaries") {
      await route.fulfill({ json: { items: [] } });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("默认主题的输入框与按钮使用统一的 8px 圆角，标题不压缩字距", async ({ page }) => {
  await page.goto("/config");
  await expect(page.getByRole("textbox", { name: "Sub2API 地址", exact: true })).toHaveCSS(
    "border-radius",
    "8px",
  );
  await expect(page.getByRole("button", { name: "保存连接", exact: true })).toHaveCSS(
    "border-radius",
    "8px",
  );
  await expect(page.getByRole("heading", { level: 1 })).toHaveCSS("letter-spacing", "normal");
});

test("320px 窄屏存在大量页码时，页容量与前后翻页按钮完整可见", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await page.goto("/logs?kind=event");
  const next = page.getByRole("button", { name: "转到下一页" });
  await expect(next).toBeEnabled();
  await expect(next).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("button", { name: "转到上一页" })).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("combobox", { name: "每页行数" })).toBeInViewport({ ratio: 1 });
  await next.click();
  await expect(page.getByLabel("当前第 2 页，共 500 页")).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("pagination.png") });
});

test("字典类型在窄屏局部滚动，键盘可到达最后一个分类且不撑宽页面", async ({ page }) => {
  await page.goto("/config");
  await page.getByRole("tab", { name: "字典管理", exact: true }).click();
  const tabs = page.getByRole("tablist", { name: "字典类型" });
  const first = tabs.getByRole("tab", { name: "平台字典", exact: true });
  await first.focus();
  await first.press("End");
  const last = tabs.getByRole("tab", { name: "监控类型", exact: true });
  await expect(last).toBeFocused();
  await expect(last).toHaveAttribute("aria-selected", "true");
  await expect(last).toBeInViewport({ ratio: 1 });
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("dictionaries.png") });
});
