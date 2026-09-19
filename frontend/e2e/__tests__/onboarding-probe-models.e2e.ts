import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

for (const batch of [false, true]) {
  test(`${batch ? "批量" : "单个"}新增账号获取并选择探活模型，提交不互相覆盖`, async ({ page }) => {
    const upstream = {
      upstream_id: "probe-models-upstream",
      host: "models.example.test",
      name: "探活模型测试上游",
      base_url: "https://models.example.test",
      account_base_url: "https://models.example.test",
      upstream_type: "sub2api",
      auth_mode: "sub2api_user_token",
      recharge_rate: "1",
      raw_balance: "10",
      balance: "10",
      has_access_token: true,
      has_refresh_token: false,
      has_admin_key: false,
      has_user_id: false,
      headers: {},
      header_names: [],
      cookie_names: [],
      groups: [],
    };
    const candidate = {
      number: 1,
      upstream_id: upstream.upstream_id,
      host: upstream.host,
      upstream_name: upstream.name,
      group_id: "7",
      group_name: "待新增分组",
      description: null,
      platform: "openai",
      status: "active",
      multiplier: "1",
      recommended_binding: "",
      bindable: true,
      can_create_key: true,
      can_bind_existing_key: false,
      bound: false,
      key_present: false,
      bound_accounts: [],
      unavailable_reason: null,
    };
    const task = {
      id: "isolated-onboard",
      skill: "onboarding",
      operation: "onboard",
      status: "queued",
      progress: 0,
      message: "排队中",
      result: {},
      created_at: "",
      updated_at: "",
    };
    const modelTask = {
      ...task,
      id: "isolated-model-options",
      operation: "onboarding-probe-model-options",
    };
    const probeRequests: string[] = [];
    const missingFixtures = new Set<string>();
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path.startsWith("/api/onboarding/probe/")) probeRequests.push(path);
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "探活模型测试" },
        "/api/preferences/navigation": { hidden_item_ids: [], version: "test" },
        "/api/policy": {
          probe: { model: "", platform_models: {} },
          advanced_policy: { group_policies: {} },
        },
        "/api/onboarding/concurrency-preview": {
          items: Array.from({ length: batch ? 2 : 1 }, () => ({ concurrency: 10 })),
        },
        "/api/overview": {
          database_available: true,
          account_count: 0,
          group_count: 2,
          open_alerts: 0,
          recent_runs: 0,
          last_activity: null,
          mode: "完全模式",
        },
        "/api/dictionaries": { items: [] },
        "/api/inspection/automation": {
          enabled: false,
          interval_seconds: 60,
          running: false,
          monitoring_configured: false,
          monitoring_enabled: false,
          monitoring_checked_at: null,
          last_run_duration_ms: 0,
          last_summary: {
            channels: 0,
            probed: 0,
            samples: 0,
            fused: 0,
            recovered: 0,
            applied: 0,
            cleaned_up: 0,
            alerts: 0,
          },
          last_run_at: null,
          next_run_at: null,
          last_status: null,
          last_error: null,
          last_task_id: null,
          queue: [],
          heartbeat_history: [],
        },
        "/api/upstreams": { hosts: [] },
        "/api/upstreams/models.example.test/configuration": upstream,
        "/api/onboarding/prepare": { upstream, candidates: [candidate] },
        "/api/groups": [
          {
            id: "4",
            name: "备用 OpenAI",
            platform: "openai",
            account_count: 0,
            platforms: ["openai"],
          },
          {
            id: "3",
            name: "本地 OpenAI",
            platform: "openai",
            account_count: 0,
            platforms: ["openai"],
          },
        ],
        "/api/onboarding": task,
        "/api/onboarding/batch": task,
        "/api/tasks/isolated-onboard": task,
        "/api/onboarding/probe/tasks/model-options": modelTask,
        "/api/tasks/isolated-model-options": {
          ...modelTask,
          status: "succeeded",
          progress: 100,
          message: "模型列表已读取",
          result: { models: ["gpt-5.2", "gpt-5.1", "gpt-5-mini"] },
        },
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else {
        missingFixtures.add(path);
        await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      }
    });
    await page.goto("/onboarding?host=models.example.test&upstream_type=sub2api&group_id=%227%22");
    const groups = page.getByRole("combobox", { name: "待新增分组 本地分组" });
    await groups.click();
    await page.getByRole("option", { name: /本地 OpenAI/ }).click();
    if (batch) await page.getByRole("option", { name: /备用 OpenAI/ }).click();
    await groups.press("Escape");
    await page.getByRole("button", { name: "预览添加账号" }).click();
    const dialog = page.getByRole("dialog", { name: "确认账号绑定变更" });
    const primary = dialog.getByRole("region", { name: "待新增分组 → 本地 OpenAI", exact: true });
    const models = primary.getByRole("combobox", { name: "待新增分组 → 本地 OpenAI 探活模型" });
    await expect(models).toBeInViewport({ ratio: 1 });
    await expect(
      dialog.getByRole("button", { name: `确认提交 ${batch ? 2 : 1} 项变更` }),
    ).toBeInViewport({
      ratio: 1,
    });
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    const [modelRequest] = await Promise.all([
      page.waitForRequest(
        (request) =>
          new URL(request.url()).pathname === "/api/onboarding/probe/tasks/model-options" &&
          request.method() === "POST",
      ),
      primary.getByRole("button", { name: "获取模型", exact: true }).click(),
    ]);
    expect(modelRequest.postDataJSON()).toEqual({
      host: upstream.host,
      group_id: candidate.group_id,
    });
    await expect(models).not.toHaveAttribute("aria-disabled", "true");
    await models.click();
    await page.getByRole("option", { name: "gpt-5.2", exact: true }).click();
    await page.getByRole("option", { name: "gpt-5.1", exact: true }).click();
    await page.getByRole("button", { name: /清空/ }).click();
    await expect(models).not.toContainText("gpt-5.2");
    await expect(models).not.toContainText("gpt-5.1");
    await models.click();
    await page.getByRole("option", { name: "gpt-5.2", exact: true }).click();
    await page.getByRole("option", { name: "gpt-5.1", exact: true }).click();
    await expect(models).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
    await expect(models).toHaveAttribute("aria-expanded", "false");
    await expect(page.getByRole("listbox")).toHaveCount(0);
    if (batch) {
      const secondary = dialog.getByRole("region", {
        name: "待新增分组 → 备用 OpenAI",
        exact: true,
      });
      const secondaryModels = secondary.getByRole("combobox", {
        name: "待新增分组 → 备用 OpenAI 探活模型",
      });
      await secondaryModels.scrollIntoViewIfNeeded();
      await secondaryModels.click();
      await page.getByRole("option", { name: "gpt-5.1", exact: true }).click();
      await page.getByRole("option", { name: "gpt-5-mini", exact: true }).click();
      await expect(secondaryModels).toHaveAttribute("aria-expanded", "true");
      await page.keyboard.press("Escape");
      await expect(dialog).toBeVisible();
      await expect(secondaryModels).toHaveAttribute("aria-expanded", "false");
      await expect(secondaryModels).not.toContainText("gpt-5.2");
    }
    await expect(models).toContainText("gpt-5.2");
    await expect(models).toContainText("gpt-5.1");
    await expect(models).not.toContainText("gpt-5-mini");
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    await page.screenshot({ path: test.info().outputPath("onboarding-probe-models.png") });
    const [request] = await Promise.all([
      page.waitForRequest(
        (request) =>
          new URL(request.url()).pathname ===
            (batch ? "/api/onboarding/batch" : "/api/onboarding") && request.method() === "POST",
      ),
      dialog.getByRole("button", { name: `确认提交 ${batch ? 2 : 1} 项变更` }).click(),
    ]);
    const payload = request.postDataJSON();
    const requests = batch ? payload.items : [payload];
    expect(
      requests.find((item: { local_group_ids: number[] }) => item.local_group_ids[0] === 3),
    ).toMatchObject({
      test_models: ["gpt-5.2", "gpt-5.1"],
      host: upstream.host,
      upstream_group_id: candidate.group_id,
      local_group_ids: [3],
    });
    if (batch)
      expect(
        requests.find((item: { local_group_ids: number[] }) => item.local_group_ids[0] === 4),
      ).toMatchObject({ test_models: ["gpt-5.1", "gpt-5-mini"], local_group_ids: [4] });
    expect(probeRequests).toEqual(["/api/onboarding/probe/tasks/model-options"]);
    expect([...missingFixtures]).toEqual([]);
  });
}
