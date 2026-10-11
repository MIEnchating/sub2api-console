import { expect, test } from "@playwright/test";
import type { Task, TaskSummary } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

for (const theme of ["light", "dark"] as const) {
  test(`运营任务详情大结果分页（${theme}）`, async ({ page }) => {
    await page.addInitScript(
      (value) => localStorage.setItem("sub2api-console-theme", value),
      theme,
    );
    const items = [
      ...Array.from({ length: 41 }, (_, index) => ({
        account_id: `account-${index + 1}`,
        status: "succeeded",
        models: ["model-a"],
      })),
      ...Array.from({ length: 41 }, (_, index) => ({
        channel_id: `channel-${index + 1}`,
        channel_name: `渠道 ${index + 1}`,
        status: "succeeded",
        added: ["model-a"],
      })),
    ];
    const detail: Task = {
      id: "automation-large",
      skill: "sub2api-account-model-automation",
      operation: "account-model-automation",
      status: "succeeded",
      progress: 100,
      message: "执行完成",
      result: {
        dry_run: false,
        result_version: 2,
        models: ["model-a"],
        pricing_plan: Array.from({ length: 41 }, (_, index) => ({
          model: `price-${index + 1}`,
          input_ratio: "0.123456789012345678",
          completion_ratio: "2",
        })),
        items,
      },
      created_at: "2026-10-10T00:00:00Z",
      updated_at: "2026-10-10T00:00:01Z",
    };
    const summary: TaskSummary = { ...detail, system_info: true };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "分页测试" },
        "/api/tasks": [summary],
        "/api/tasks/automation-large": detail,
        "/api/dictionaries": { items: [] },
        "/api/system/metrics": {
          cpu: { usage_percent: 10, logical_cores: 4 },
          memory: { usage_percent: 20, used_bytes: 1024, total_bytes: 4096 },
          disk: { usage_percent: 30, used_bytes: 1024, total_bytes: 4096 },
        },
        "/api/inspection/automation": {
          enabled: false,
          running: false,
          traffic_collection: { enabled: false },
        },
      };
      if (path.endsWith("/events")) {
        return route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      }
      if (route.request().method() === "GET" && path in fixtures) {
        return route.fulfill({ json: fixtures[path] });
      }
      return route.fulfill({ status: 404, json: { detail: "未配置的隔离接口" } });
    });

    await page.goto("/system-info");
    await page.getByRole("tab", { name: "历史任务 1" }).click();
    await page.getByRole("button", { name: "查看任务", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "任务详情", exact: true });
    const result = dialog.getByRole("region", { name: "运营执行结果" });
    const accounts = result.getByRole("region", { name: "账号模型目录" });
    const channels = result.getByRole("region", { name: "渠道模型上架" });
    const prices = result.getByRole("region", { name: "模型定价方案" });
    await expect(accounts.getByRole("listitem")).toHaveCount(20);
    await expect(channels.getByRole("listitem")).toHaveCount(20);
    await expect(prices.locator(".border-b")).toHaveCount(20);
    await expect(accounts.getByText("账号 account-21", { exact: false })).toHaveCount(0);
    await accounts.getByRole("button", { name: "转到下一页" }).click();
    await channels.getByRole("button", { name: "转到下一页" }).click();
    await prices.getByRole("button", { name: "转到下一页" }).click();
    await expect(accounts.getByText("账号 account-21", { exact: false })).toBeVisible();
    await expect(channels.getByText("渠道 21", { exact: false })).toBeVisible();
    await expect(prices.getByText("price-21", { exact: true })).toBeVisible();
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
  });
}
