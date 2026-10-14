import { expect, test } from "@playwright/test";

import type { Task } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

const upstream = {
  upstream_id: "cleanup",
  host: "cleanup.example.test",
  name: "清理测试上游",
  base_url: "https://cleanup.example.test",
  account_base_url: "https://cleanup.example.test",
  upstream_type: "sub2api",
  auth_mode: "access_token",
  recharge_rate: "1",
  balance: "10",
  groups: [],
  headers: {},
  header_names: [],
  cookie_names: [],
  has_access_token: true,
};

const longName = "超长上游 Key 名称".repeat(20);
const longReason = "远端已经建立绑定，保留该 Key。".repeat(20);

for (const viewport of [
  { width: 1440, height: 900 },
  { width: 390, height: 664 },
  { width: 320, height: 568 },
]) {
  test(`${viewport.width}px 清理分页提交全部 Key，实时结果和取消操作保持可用`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.emulateMedia({ colorScheme: viewport.width < 640 ? "dark" : "light" });
    const errors: Error[] = [];
    page.on("pageerror", (error) => errors.push(error));
    const keys = Array.from({ length: 500 }, (_, index) => ({
      key_id: String(index + 1),
      name: index === 0 ? longName : `待清理 Key ${index + 1}`,
      group_id: "6",
      status: "active",
    }));
    let task: Task = {
      id: "cleanup-task",
      skill: "sub2api-account-onboarding",
      operation: "upstream-key-cleanup",
      status: "running",
      progress: 4,
      message: "正在清理 Key",
      result: {
        deleted: 20,
        skipped: 1,
        failed: 0,
        completed: 21,
        total: 500,
        items: keys.slice(0, 21).map((key, index) => ({
          key_id: key.key_id,
          name: key.name,
          status: index === 0 ? "skipped" : "deleted",
          reason: index === 0 ? longReason : "",
        })),
      },
      created_at: "2026-10-10T00:00:00Z",
      updated_at: "2026-10-10T00:01:00Z",
    };
    let submitted: unknown;
    let cancelled = false;
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "清理分页测试" },
        "/api/overview": {
          database_available: true,
          account_count: 0,
          group_count: 0,
          open_alerts: 0,
          recent_runs: 0,
          last_activity: null,
          mode: "完全模式",
        },
        "/api/inspection/automation": {
          enabled: false,
          running: false,
          traffic_collection: { enabled: false },
        },
        "/api/catalog/groups": [],
        "/api/catalog/upstreams": [],
        "/api/dictionaries": { items: [] },
        "/api/groups": [],
        "/api/upstreams/cleanup.example.test/configuration": upstream,
        "/api/onboarding/prepare": { upstream, candidates: [] },
        "/api/onboarding/keys/cleanup-preview": { host: upstream.host, keys },
      };
      if (path === "/api/onboarding/keys/cleanup") {
        submitted = route.request().postDataJSON();
        await route.fulfill({ json: task });
      } else if (path === "/api/tasks/cleanup-task" && route.request().method() === "DELETE") {
        cancelled = true;
        task = { ...task, status: "cancelled", progress: 100, message: "已取消 Key 清理" };
        await route.fulfill({ json: { cancelled: true } });
      } else if (path === "/api/tasks/cleanup-task") {
        await route.fulfill({ json: task });
      } else if (path.endsWith("/events")) {
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      } else if (path in fixtures) {
        await route.fulfill({ json: fixtures[path] });
      } else {
        await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      }
    });

    await page.goto(`/onboarding?host=${upstream.host}`);
    await page.getByRole("button", { name: "清理无用 Key" }).click();
    const dialog = page.getByRole("dialog", { name: "清理无绑定上游 Key" });
    const preview = dialog.getByRole("table", { name: "待清理 Key", exact: true });
    await expect(preview.getByRole("row")).toHaveCount(21);
    await expect(preview.getByRole("cell", { name: longName, exact: true })).toBeVisible();
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    const container = dialog.locator('[data-slot="table-container"]');
    expect(await container.evaluate((element) => getComputedStyle(element).overflowX)).toBe("auto");
    if (viewport.width < 640) {
      expect(await container.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
        true,
      );
    }
    await dialog.getByRole("button", { name: "转到下一页" }).focus();
    await page.keyboard.press("Enter");
    await expect(preview.getByRole("cell", { name: "待清理 Key 21", exact: true })).toBeVisible();
    await expect(preview.getByRole("cell", { name: longName, exact: true })).toHaveCount(0);
    const confirm = dialog.getByRole("button", { name: "确认删除 500 个 Key" });
    await expect(confirm).toBeInViewport({ ratio: 1 });
    await confirm.click();
    await expect
      .poll(() => submitted)
      .toEqual({ host: upstream.host, key_ids: keys.map((key) => key.key_id) });

    const results = dialog.getByRole("table", { name: "Key 清理结果", exact: true });
    await expect(results.getByRole("row")).toHaveCount(21);
    await expect(results.getByRole("cell", { name: longReason, exact: true })).toBeAttached();
    await expect(dialog.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "4");
    await expect(dialog.getByRole("group", { name: "Key 清理统计" })).toContainText("已跳过");
    await dialog.getByRole("button", { name: "转到下一页" }).click();
    await expect(results.getByRole("cell", { name: "待清理 Key 21", exact: true })).toBeVisible();
    const originalTable = await results.elementHandle();
    task = {
      ...task,
      result: {
        ...task.result,
        deleted: 21,
        completed: 22,
        items: [
          ...(task.result.items as object[]),
          { key_id: "22", name: "待清理 Key 22", status: "deleted" },
        ],
      },
    };
    await expect(results.getByRole("cell", { name: "待清理 Key 22", exact: true })).toBeVisible();
    expect(await originalTable!.evaluate((element) => element.isConnected)).toBe(true);
    await dialog.getByRole("button", { name: "取消任务" }).click();
    await expect.poll(() => cancelled).toBe(true);
    await expect(dialog.getByRole("button", { name: "完成" })).toBeInViewport({ ratio: 1 });
    await expect(dialog.getByRole("progressbar")).toHaveCount(0);
    await expect(results.getByRole("cell", { name: "待清理 Key 22", exact: true })).toBeVisible();
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await page.screenshot({ path: test.info().outputPath(`key-cleanup-${viewport.width}.png`) });
    expect(errors).toEqual([]);
  });
}
