import { expect, test } from "@playwright/test";
import type { AccountModelAutomationSchedule, NewAPIChannel } from "../../src/api";
import { pageFixtures, workspace } from "./fixtures/page-shell";

function channel(id: string): NewAPIChannel {
  return {
    id,
    name: `渠道 ${id}`,
    type: 1,
    status: 1,
    models: ["model-a"],
    groups: [],
    version: "a".repeat(64),
  };
}

test("大目录编辑按ID恢复已选项，翻页选择和保存保持完整范围", async ({ page }) => {
  const schedule: AccountModelAutomationSchedule = {
    id: "paged",
    name: "跨页任务",
    enabled: true,
    interval_minutes: 60,
    last_run_at: null,
    next_run_at: null,
    request: {
      platform_id: "layout",
      group_ids: ["7"],
      channels: [{ id: "1201", version: channel("1201").version }],
      selected_models: ["model-a"],
      publish_missing: true,
      price_enabled: false,
      pricing_source: "official",
    },
  };
  const pages = new Set<number>();
  let saved: unknown;
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path.endsWith("/channels/lookup")) {
      await route.fulfill({
        json: { items: url.searchParams.getAll("id").map(channel), failures: [] },
      });
      return;
    }
    if (path.endsWith("/channels")) {
      const index = Number(url.searchParams.get("page"));
      const size = Number(url.searchParams.get("page_size"));
      pages.add(index);
      await route.fulfill({
        json: {
          items: Array.from({ length: Math.min(size, 1301 - index * size) }, (_, i) =>
            channel(String(index * size + i + 1)),
          ),
          total: 1301,
        },
      });
      return;
    }
    if (path.endsWith("/automation/schedules") && route.request().method() === "PUT") {
      saved = route.request().postDataJSON();
      await route.fulfill({ json: schedule });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离测试" },
      "/api/newapi": {
        ...workspace,
        bindings: [
          {
            platform_id: "layout",
            sub2api_group_id: "7",
            newapi_group_id: "7",
            newapi_group_name: "测试分组",
            sync_ratio: true,
          },
        ],
      },
      "/api/catalog/group-memberships": [{ id: "7", name: "测试分组", account_count: 1 }],
      "/api/management/accounts/models/automation/schedules": [schedule],
      "/api/management/accounts/models/automation/task-groups": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/newapi/account-model-automation");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑运营任务" });
  const channels = dialog.getByRole("group", { name: "目标渠道", exact: true });
  await expect(channels.getByRole("checkbox", { name: "渠道 1201（1201）" })).toBeChecked();
  await expect(dialog.getByRole("button", { name: "保存任务", exact: true })).toBeEnabled();
  expect([...pages]).toEqual([0]);
  await channels.getByRole("button", { name: "转到下一页" }).click();
  const next = channels.getByRole("checkbox", { name: "渠道 51（51）" });
  await expect(next).toBeEnabled();
  await next.check();
  await expect(channels.getByRole("checkbox", { name: "渠道 1201（1201）" })).toBeChecked();
  await expect(channels.getByRole("checkbox", { name: "渠道 1（1）" })).toHaveCount(0);
  expect([...pages]).toEqual([0, 1]);
  const pagination = channels.getByRole("navigation", { name: "表格分页" });
  await pagination.scrollIntoViewIfNeeded();
  await expect(pagination.getByRole("button", { name: "转到上一页" })).toBeInViewport();
  await expect(pagination.getByRole("button", { name: "转到下一页" })).toBeInViewport();
  await pagination.getByRole("button", { name: "转到上一页" }).click();
  await expect(channels.getByRole("checkbox", { name: "渠道 1（1）" })).toBeEnabled();
  await expect(channels.getByRole("checkbox", { name: "渠道 51（51）" })).toBeChecked();
  await pagination.getByRole("button", { name: "转到下一页" }).click();
  await expect(channels.getByRole("checkbox", { name: "渠道 52（52）" })).toBeEnabled();
  await expect(channels.getByRole("checkbox", { name: "渠道 51（51）" })).toBeChecked();
  await pagination.scrollIntoViewIfNeeded();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("paged-selection.png") });
  await dialog.getByRole("button", { name: "保存任务", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(saved).toMatchObject({
    channels: [
      { id: "1201", version: channel("1201").version },
      { id: "51", version: channel("51").version },
    ],
  });
});
