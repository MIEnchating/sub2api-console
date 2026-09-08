import { expect, test } from "@playwright/test";
import type { GroupPolicyOverrideUpdate, GroupStatus } from "../../src/api";

test("继承全局策略时保存和重新打开保留探活模型，空列表也能切换模型选择", async ({ page }) => {
  let saved: GroupPolicyOverrideUpdate | null = null;
  const group: GroupStatus = {
    id: "6",
    name: "探活模型回归分组",
    account_count: 1,
    scheduling_open: 1,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "speed_first",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
    override: { probe_model: "saved-model" },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/groups/6/policy" && route.request().method() === "PUT") {
      saved = route.request().postDataJSON() as GroupPolicyOverrideUpdate;
      group.override = saved;
      await route.fulfill({ json: group });
      return;
    }
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "分组回归测试" },
      "/api/config": { mode: "完全模式", values: {} },
      "/api/overview": {
        database_available: true,
        account_count: 1,
        group_count: 1,
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
      "/api/groups": [group],
      "/api/policy": {
        available: true,
        global_strategy: "speed_first",
        advanced_policy: {},
        probe_interval_seconds: 300,
        probe_model: "global-model",
        configuration_errors: [],
      },
      "/api/groups/6/models": {
        group_id: "6",
        group_name: group.name,
        models: [],
        account_count: 1,
        accounts_with_models: 0,
        complete: false,
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (route.request().method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/groups");
  await page.getByRole("button", { name: "编辑分组", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑分组策略" });
  await expect(dialog.getByRole("radio", { name: "全局默认" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  const input = dialog.getByRole("textbox", { name: "手动输入探活模型" });
  await expect(input).toHaveValue("saved-model");
  await input.fill("");
  await expect(dialog.getByText("当前继承全局模型：global-model")).toBeVisible();
  await dialog.getByRole("button", { name: "选择模型", exact: true }).click();
  const select = dialog.getByRole("combobox", { name: "选择探活模型" });
  await expect(select).toContainText("继承全局默认模型");
  await expect(dialog.getByRole("status")).toHaveText("暂无组内共同模型，可手动输入探活模型。");
  await select.click();
  await page.getByRole("option", { name: "继承全局默认模型" }).click();
  await dialog.getByRole("button", { name: "手动输入", exact: true }).click();
  await input.fill("saved-model");
  await dialog.getByRole("button", { name: "保存策略", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(saved).toMatchObject({ strategy: null, probe_model: "saved-model" });
  await page.getByRole("button", { name: "编辑分组", exact: true }).click();
  await expect(input).toHaveValue("saved-model");
  await expect(dialog.getByRole("radio", { name: "全局默认" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  await page.route("**/api/groups/6/models", (route) =>
    route.fulfill({
      json: {
        group_id: "6",
        group_name: group.name,
        models: ["gpt-5.2"],
        account_count: 1,
        accounts_with_models: 1,
        complete: true,
      },
    }),
  );
  await dialog.getByRole("button", { name: "重新获取组内模型" }).click();
  await expect(dialog.getByRole("status")).toHaveCount(0);
  await dialog.getByRole("button", { name: "选择模型", exact: true }).click();
  await expect(select).toContainText("saved-model");
  await select.click();
  await page.getByRole("option", { name: "gpt-5.2" }).click();
  await dialog.getByRole("button", { name: "保存策略", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(saved).toMatchObject({ strategy: null, probe_model: "gpt-5.2" });
  await page.getByRole("button", { name: "编辑分组", exact: true }).click();
  await expect(input).toHaveValue("gpt-5.2");
  await page.screenshot({ path: test.info().outputPath("group-policy-editor.png") });
});
