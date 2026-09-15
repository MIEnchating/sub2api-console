import { expect, test, type Page } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { policy } from "./fixtures/settings";

const accountSettings = {
  default: {
    models: ["model-a"],
    concurrency: 10,
    load_factor: null,
    priority: 1,
    pool_mode: false,
    pool_mode_retry_count: 3,
    pool_mode_retry_status_codes: [429],
  },
  groups: [],
  platform_probe_models: {},
};
const alert = {
  incident_key: "incident-1",
  event_type: "account.probe",
  object_kind: "account",
  object_id: "41",
  object_name: "缓存告警账号",
  cause_code: "PROBE",
  status: "recovered",
  first_seen_at: "2026-09-01T08:00:00Z",
  last_seen_at: "2026-09-01T08:10:00Z",
  last_error: null,
  delivery_status: "sent",
  delivery_attempts: 1,
  delivered_at: null,
};

async function mockSettings(page: Page): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "加载测试" },
      "/api/config/account-settings": accountSettings,
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/policy": policy,
      "/api/alerts": [alert],
      "/api/dictionaries": {
        items: [
          {
            id: "openai",
            kind: "platform",
            name: "OpenAI",
            value: "openai",
            description: "平台",
            enabled: true,
            sort_order: 0,
          },
        ],
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
}

const layouts = [
  {
    tab: "connection",
    title: "Sub2API 连接",
    count: 2,
    endpoint: "/api/config",
    loaded: "保存连接",
  },
  {
    tab: "accounts",
    title: "账号设置",
    count: 2,
    endpoint: "/api/config/account-settings",
    loaded: "保存全局默认",
  },
  {
    tab: "notifications",
    title: "QQBot 通知接入",
    count: 1,
    endpoint: "/api/notifications/status",
    loaded: "保存通知设置",
  },
  {
    tab: "interface",
    title: "日志保留",
    count: 2,
    endpoint: "/api/config/log-cleanup",
    loaded: "保存日志设置",
  },
  { tab: "dictionaries", title: "字典管理", count: 1, endpoint: "/api/dictionaries", loaded: "" },
];

for (const scenario of layouts) {
  test(`${scenario.tab} 设置首次加载、父配置返回及内容完成使用同一布局`, async ({
    page,
    viewport,
  }) => {
    await mockSettings(page);
    let releaseConfig!: () => void;
    let releaseData!: () => void;
    const configGate = new Promise<void>((resolve) => {
      releaseConfig = resolve;
    });
    const dataGate = new Promise<void>((resolve) => {
      releaseData = resolve;
    });
    let configPending = true;
    let dataPending = true;
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/config" && configPending) await configGate;
      if (path === scenario.endpoint && dataPending) await dataGate;
      await route.fallback();
    });
    try {
      await page.goto(`/config?tab=${scenario.tab}`);
      const panel = page.getByTestId("system-settings-panel");
      const cards = panel.locator('[data-slot="card"]');
      await expect(cards).toHaveCount(scenario.count);
      await expect(panel.getByText(scenario.title, { exact: true })).toBeVisible();
      const before = await cards.evaluateAll((elements) =>
        elements.map((element) => {
          const box = element.getBoundingClientRect();
          return { x: box.x, y: box.y, width: box.width, height: box.height };
        }),
      );
      if (scenario.count === 2 && viewport!.width >= 1280) {
        expect(before[0].width).toBeGreaterThan(before[1].width);
        expect(Math.abs(before[1].y - before[0].y)).toBeLessThanOrEqual(2);
      }
      if (scenario.count === 2 && viewport!.width < 1280)
        expect(before[1].y).toBeGreaterThanOrEqual(before[0].y + before[0].height);
      configPending = false;
      releaseConfig();
      if (scenario.tab !== "connection") {
        await expect(page.getByRole("button", { name: "刷新系统设置" })).toBeEnabled();
        await expect(cards).toHaveCount(scenario.count);
        await expect(panel.locator('[data-slot="skeleton"]').first()).toBeVisible();
      }
      dataPending = false;
      releaseData();
      if (scenario.loaded)
        await expect(
          page.getByRole("button", { name: scenario.loaded, exact: true }),
        ).toBeAttached();
      else await expect(panel.getByText("OpenAI", { exact: true })).toBeVisible();
      const after = await cards.evaluateAll((elements) =>
        elements.map((element) => {
          const box = element.getBoundingClientRect();
          return { x: box.x, y: box.y, width: box.width, height: box.height };
        }),
      );
      for (let index = 0; index < before.length; index += 1) {
        expect(Math.abs(after[index].x - before[index].x)).toBeLessThanOrEqual(2);
        expect(Math.abs(after[index].width - before[index].width)).toBeLessThanOrEqual(2);
        expect(Math.abs(after[index].y - before[index].y)).toBeLessThanOrEqual(2);
      }
      expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
        true,
      );
    } finally {
      releaseConfig();
      releaseData();
    }
  });
}

test("策略刷新失败后保留输入草稿并禁止保存", async ({ page }) => {
  await mockSettings(page);
  let failed = false;
  await page.route("**/api/policy", (route) =>
    failed
      ? route.fulfill({ status: 503, json: { detail: "策略刷新暂不可用" } })
      : route.fallback(),
  );
  await page.goto("/policy");
  const budget = page.getByRole("spinbutton", { name: "每组总权重预算", exact: true });
  await budget.fill("500");
  failed = true;
  await page.getByRole("button", { name: "刷新策略" }).click();
  await expect(
    page.locator("[data-sonner-toast]").filter({ hasText: "策略刷新暂不可用" }),
  ).toBeVisible({ timeout: 15000 });
  await expect(budget).toHaveValue("500");
  await expect(page.getByRole("button", { name: "保存策略", exact: true })).toBeDisabled();
});

test("告警刷新失败后保留告警列表及分页并禁止清理", async ({ page }) => {
  await mockSettings(page);
  let failed = false;
  await page.route("**/api/alerts?*", (route) =>
    failed
      ? route.fulfill({ status: 503, json: { detail: "告警刷新暂不可用" } })
      : route.fallback(),
  );
  await page.goto("/alerts");
  await expect(page.getByText(/缓存告警账号/)).toBeVisible();
  failed = true;
  await page.getByRole("button", { name: "刷新告警" }).click();
  await expect(
    page.locator("[data-sonner-toast]").filter({ hasText: "告警刷新暂不可用" }),
  ).toBeVisible({ timeout: 15000 });
  await expect(page.getByText(/缓存告警账号/)).toBeVisible();
  await expect(page.getByRole("navigation", { name: "表格分页" })).toBeVisible();
  await expect(page.getByRole("button", { name: "清理已结束" })).toBeDisabled();
});

test("连接页概要刷新失败后保留已读取统计", async ({ page }) => {
  await mockSettings(page);
  let failed = false;
  const summary = {
    enabled: false,
    running: false,
    traffic_collection: { enabled: false },
    last_run_at: "2026-09-01T08:00:00Z",
    last_status: "succeeded",
    last_run_duration_ms: 5000,
    last_summary: {
      channels: 7,
      probed: 3,
      samples: 12,
      fused: 0,
      recovered: 1,
      applied: 2,
      cleaned_up: 0,
      alerts: 0,
    },
  };
  await page.route("**/api/inspection/automation", (route) =>
    route.fulfill({
      status: failed ? 503 : 200,
      json: failed ? { detail: "概要刷新暂不可用" } : summary,
    }),
  );
  await page.goto("/config");
  const values = page.getByTestId("inspection-summary-grid");
  await expect(values).toContainText("受管账号7");
  failed = true;
  // Returning to the connection tab starts its enabled query again without discarding cache.
  await page.getByRole("tab", { name: "通知设置", exact: true }).click();
  await page.getByRole("tab", { name: "连接设置", exact: true }).click();
  await expect(
    page.locator("[data-sonner-toast]").filter({ hasText: "概要刷新暂不可用" }),
  ).toBeVisible({ timeout: 25000 });
  await expect(values).toContainText("受管账号7");
});
