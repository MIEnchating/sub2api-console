import { expect, test } from "@playwright/test";
import type { PolicySnapshot } from "../../src/api";

const policy: PolicySnapshot = {
  revision: "isolated-policy",
  available: true,
  source: "隔离测试",
  mode: "完全模式",
  global_strategy: "balanced",
  group_strategies: [],
  missing_rate_fallback: "current_cost_wall",
  change_threshold: "0.1",
  cooldown_seconds: 60,
  auto_apply: { schedulable: true, priority: true, load_factor: true, concurrency: false },
  excluded_group_ids: [],
  traffic_enabled: true,
  probe_interval_seconds: 300,
  probe_model: "test-model",
  traffic_lookback_minutes: 120,
  max_samples_per_account: 60,
  advanced_policy: {
    upstream_multiplier: { interval_seconds: 120 },
    traffic: { refresh_seconds: 60 },
    scoring: {
      short_window: 10,
      long_window: 60,
      latest_weight: 0.5,
      short_ratio: 0.7,
      slow_ttfb_ms: 5000,
    },
  },
  configuration_errors: [],
};

test("独立编辑新鲜度与历史窗口后，保存保留探测间隔及样本条数", async ({ page }) => {
  let saved: Record<string, unknown> | undefined;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/policy" && route.request().method() === "PUT") {
      saved = route.request().postDataJSON() as Record<string, unknown>;
      await route.fulfill({ json: policy });
      return;
    }
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/policy": policy,
      "/api/config": { probes_enabled: true },
      "/api/groups": [],
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/policy");
  await page.getByRole("button", { name: "巡检与采样", exact: true }).click();
  const freshness = page.getByRole("spinbutton", { name: "当前探针有效期（秒）" });
  await expect(freshness).toHaveValue("900");
  await freshness.fill("1800");
  await page.getByRole("button", { name: "健康与处置", exact: true }).click();
  const history = page.getByRole("spinbutton", { name: "评分历史范围（分钟）" });
  await expect(history).toHaveValue("1440");
  await history.fill("720");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await expect
    .poll(() => saved)
    .toMatchObject({
      probe_interval_seconds: 300,
      advanced_policy: {
        probe: { freshness_seconds: 1800 },
        scoring: { history_window_minutes: 720, short_window: 10, long_window: 60 },
      },
    });
});
