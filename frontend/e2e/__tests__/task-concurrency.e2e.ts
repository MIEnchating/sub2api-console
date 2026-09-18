import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { taskPoolLabels } from "../../src/features/config/constants";

test("任务并发页在桌面与手机显示独立模块，保存操作可用且布局不溢出", async ({ page }) => {
  let limit = 8;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/config/task-concurrency") {
      if (route.request().method() === "PUT") limit = route.request().postDataJSON().limits.account;
      return route.fulfill({
        json: {
          pools: Object.keys(taskPoolLabels).map((id) => ({
            id,
            limit: id === "account" ? limit : 4,
            queue_capacity: 100,
            running: 0,
            waiting: 0,
          })),
          queue_capacity: 100,
          version: String(limit),
        },
      });
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "并发布局测试" },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "test" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    if (route.request().method() === "GET" && path in fixtures)
      return route.fulfill({ json: fixtures[path] });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
  await page.goto("/config?tab=tasks");
  await expect(page.getByRole("tab", { name: "任务并发" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  const input = page.getByRole("spinbutton", { name: "账号操作并发上限" });
  await expect(input).toHaveValue("8");
  await input.fill("12");
  await page.getByRole("button", { name: "保存任务并发" }).click();
  await expect(page.getByText("任务并发设置已保存并生效")).toBeVisible();
  await expect(page.getByRole("button", { name: "保存任务并发" })).toBeDisabled();
  await page.getByRole("spinbutton", { name: "后台维护并发上限" }).scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("task-concurrency.png") });
});
