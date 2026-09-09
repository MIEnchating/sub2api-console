import { expect, test } from "@playwright/test";

import { policy } from "./fixtures/settings";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/policy": policy,
      "/api/accounts": [],
      "/api/groups": [],
      "/api/tasks": [],
      "/api/system/metrics": {
        cpu: { usage_percent: 20, logical_cores: 4 },
        memory: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
        disk: { usage_percent: 40, used_bytes: 1024, total_bytes: 4096 },
      },
      "/api/inspection/automation": {
        enabled: false,
        interval_seconds: 15,
        running: false,
        traffic_collection: { enabled: false },
        monitoring_enabled: false,
        monitoring_configured: true,
        queue: [],
        heartbeat_history: [],
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (
      path in fixtures &&
      (route.request().method() === "GET" || path === "/api/newapi/platforms/layout/refresh")
    ) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("策略表单滚动并聚焦字段时，分类栏不进入表单滚动区或覆盖字段", async ({ page }) => {
  await page.goto("/policy");
  const tabs = page.getByRole("tablist", { name: "策略分类" });
  const content = page.locator('[data-slot="page-content"]');
  await expect(tabs).toBeVisible();
  expect(await content.getByRole("tablist", { name: "策略分类" }).count()).toBe(0);
  await content.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(tabs).toBeInViewport({ ratio: 1 });
  const budget = page.getByRole("spinbutton", { name: "每组总权重预算", exact: true });
  await budget.focus();
  await expect(budget).toBeInViewport({ ratio: 1 });
  const tabBounds = (await tabs.boundingBox())!;
  expect((await budget.boundingBox())!.y).toBeGreaterThanOrEqual(tabBounds.y + tabBounds.height);
});

test("平台配置在窄屏完整显示标题，操作按钮不挤压标题", async ({ page }) => {
  await page.route("**/api/newapi", (route) =>
    route.fulfill({
      json: { platforms: [], local_groups: [], bindings: [], sub2api_base_url: "" },
    }),
  );
  await page.goto("/newapi");
  const title = page.getByRole("heading", { name: "New API 平台配置", exact: true });
  await expect(title).toBeVisible();
  expect(await title.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(
    page.getByRole("button", { name: "添加平台配置", exact: true }).first(),
  ).toBeInViewport({ ratio: 1 });
});

test("低高度窗口中系统资源卡片下的任务区域可滚动到达", async ({ page, viewport }) => {
  await page.setViewportSize({ width: viewport!.width, height: 480 });
  await page.goto("/system-info");
  await expect(page.getByText("CPU 占用", { exact: true })).toBeVisible();
  const empty = page.getByRole("cell", { name: "当前没有进行中的任务" });
  await empty.scrollIntoViewIfNeeded();
  const bounds = (await empty.boundingBox())!;
  const contentBounds = (await page.locator('[data-slot="page-content"]').boundingBox())!;
  expect(bounds.y).toBeGreaterThanOrEqual(contentBounds.y);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(contentBounds.y + contentBounds.height);
  await expect(page.getByRole("button", { name: "刷新系统信息" })).toBeInViewport({ ratio: 1 });
});

const pages = [
  ["/auto-inspection", "自动巡检"],
  ["/model-check", "模型检测"],
  ["/trace", "请求追踪"],
  ["/alerts", "告警通知"],
  ["/newapi", "New API 平台配置"],
  ["/newapi/groups", "分组绑定"],
  ["/newapi/channels", "渠道管理"],
  ["/newapi/differences", "价格比对"],
  ["/system-info", "系统信息"],
  ["/vault", "密码箱"],
  ["/logs", "日志中心"],
  ["/config", "系统设置"],
] as const;

for (const [path, title] of pages) {
  test(`${title}在桌面和窄屏中不横向溢出，滚动内容后标题完整可见`, async ({ page, viewport }) => {
    const pageErrors: string[] = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));
    await page.goto(path);
    const heading = page.getByRole("heading", { name: title, exact: true });
    await expect(heading).toBeVisible();
    const content = page.locator('[data-slot="page-content"]');
    await expect(page.locator('[data-slot="skeleton"]')).toHaveCount(0);
    expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    expect(await heading.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    if (viewport!.width >= 1280) {
      expect(
        await content.evaluate((element) => element.scrollHeight <= element.clientHeight),
      ).toBe(true);
    }
    await content.evaluate((element) => element.scrollTo(0, element.scrollHeight));
    await expect(heading).toBeInViewport({ ratio: 1 });
    if (path === "/config") {
      await expect(page.getByRole("navigation", { name: "系统设置分类导航" })).toBeInViewport({
        ratio: 1,
      });
    }
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(
      true,
    );
    expect(pageErrors).toEqual([]);
    await content.evaluate((element) => element.scrollTo(0, 0));
    await page.screenshot({ path: test.info().outputPath("page.png") });
  });
}

test("平台配置编辑时字段和底部操作可达，取消后保留原配置", async ({ page }) => {
  await page.goto("/newapi");
  await page.getByRole("button", { name: "编辑平台配置", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑 New API 平台配置" });
  await expect(dialog.getByRole("textbox", { name: "平台名称" })).toHaveValue("测试平台");
  await dialog.getByRole("textbox", { name: "平台名称" }).fill("临时草稿");
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.getByText("临时草稿", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "New API 平台配置", exact: true })).toBeInViewport(
    { ratio: 1 },
  );
});

test("分组绑定在横向滚动后可选择分组并编辑倍率，保存入口保持可达", async ({ page }) => {
  await page.goto("/newapi/groups");
  await page.getByRole("combobox", { name: "默认 的 Sub2API 分组", exact: true }).click();
  await page.getByRole("option", { name: "标准 · 1", exact: true }).click();
  const ratio = page.getByRole("textbox", { name: "默认 的 Sub2API 管理平台倍率", exact: true });
  await expect(ratio).toBeEnabled();
  await ratio.fill("1.5");
  await expect(ratio).toHaveAttribute("aria-invalid", "false");
  const save = page.getByRole("button", { name: "保存绑定与倍率", exact: true });
  await save.scrollIntoViewIfNeeded();
  await expect(save).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("heading", { name: "分组绑定", exact: true })).toBeInViewport({
    ratio: 1,
  });
});

test("价格比对选择上游和模型后显示结果，过滤无匹配项时保留筛选操作", async ({ page }) => {
  const model = { model: "test-model", input_ratio: "0.5", completion_ratio: "4" };
  await page.route("**/api/newapi/platforms/layout/refresh", (route) =>
    route.fulfill({
      json: {
        groups: [],
        models: [model],
        unset_models: [],
        references: [],
        tool_prices: [],
        differences: [],
        upstream_prices: [
          {
            host: "upstream.example.test",
            name: "比对上游",
            upstream_type: "sub2api",
            models: [model],
          },
        ],
      },
    }),
  );
  await page.goto("/newapi/differences");
  await page.getByRole("combobox", { name: "比对上游", exact: true }).click();
  await page.getByRole("option", { name: /比对上游/ }).click();
  await page.getByRole("checkbox", { name: "选择当前页全部模型" }).check();
  await page.getByRole("button", { name: "批量比对", exact: true }).click();
  await expect(page.getByLabel("test-model 比对结果")).toContainText("一致");
  await page.getByRole("textbox", { name: "搜索比对模型" }).fill("missing-model");
  await expect(page.getByRole("row").filter({ hasText: "test-model" })).toHaveCount(0);
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await expect(page.getByRole("button", { name: "批量比对", exact: true })).toBeInViewport({
    ratio: 1,
  });
});
