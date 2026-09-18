import { expect, test } from "@playwright/test";

import { policy } from "./fixtures/settings";

test("暂停时段可保存并重新读取，桌面和移动端字段均不溢出", async ({ page }) => {
  let current = structuredClone(policy);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/policy" && route.request().method() === "PUT") {
      const patch = route.request().postDataJSON();
      expect(patch.advanced_policy.probe.pause_window).toEqual({
        enabled: true,
        start: "23:00",
        end: "08:00",
        timezone: "Asia/Shanghai",
      });
      current = { ...current, ...patch, revision: "saved-pause" };
      await route.fulfill({ json: current });
      return;
    }
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "暂停时段测试" },
      "/api/policy": current,
      "/api/config": { probes_enabled: true },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (path.startsWith("/api/dictionaries")) {
      await route.fulfill({ json: { items: [] } });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/policy");
  await page.getByRole("tab", { name: "巡检与采样", exact: true }).click();
  const group = page.getByRole("group", { name: "自动探活暂停时段" });
  await group.scrollIntoViewIfNeeded();
  await expect(page.getByLabel("暂停开始时间")).toBeDisabled();
  await page.getByRole("switch", { name: "启用每日探活暂停时段" }).click();
  await page.getByLabel("暂停开始时间").fill("23:00");
  await expect(page.getByLabel("暂停时区")).toHaveValue("Asia/Shanghai");
  expect(await group.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("probe-pause.png") });
  const saved = page.waitForResponse(
    (response) => response.url().endsWith("/api/policy") && response.request().method() === "PUT",
  );
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await saved;
  await page.reload();
  await page.getByRole("tab", { name: "巡检与采样", exact: true }).click();
  await expect(page.getByRole("switch", { name: "启用每日探活暂停时段" })).toBeChecked();
  await expect(page.getByLabel("暂停开始时间")).toHaveValue("23:00");
});
