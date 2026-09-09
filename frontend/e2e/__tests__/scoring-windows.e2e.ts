import { expect, test } from "@playwright/test";
import { policy } from "./fixtures/settings";

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
  await page.getByRole("tab", { name: "巡检与采样", exact: true }).click();
  const freshness = page.getByRole("spinbutton", { name: "当前探针有效期（秒）" });
  await expect(freshness).toHaveValue("900");
  await freshness.fill("1800");
  await page.getByRole("tab", { name: "健康与处置", exact: true }).click();
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
