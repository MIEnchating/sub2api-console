import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { dictionaryResponse } from "./fixtures/dictionaries";

for (const detectionMode of ["verification"] as const) {
  test(`添加账号 ${detectionMode} 沿用模型且报告状态与能力结论独立`, async ({
    page,
    colorScheme,
  }) => {
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
      colorScheme,
    );
    let finished = false;
    let finishing = false;
    const unhandled: string[] = [];
    const model = "selected-model-" + "long-model-name-".repeat(8);
    const report = {
      checker: "verification",
      account_name: "接入前探活",
      claimed_model: model,
      total: 29,
      public_reference: {
        version: "public-fingerprint-v1",
        model,
        status: "inconclusive",
        summary: "分词一致仅是辅助证据，无法确认具体型号。",
        tokenizer: {
          status: "consistent",
          samples: [{ id: "中文", reference: 51, observed: 51, status: "consistent" }],
        },
        sources: [],
      },
      capability_total: 7,
      passed: 1,
      capability: { status: "failed", summary: "答案未通过" },
      authenticity: { status: "inconclusive", summary: "第三方接口" },
      evidence: [],
      probes: Array.from({ length: 7 }, (_, index) => ({
        id: `question-${index}`,
        label: `推理题 ${index + 1}`,
        round: 1,
        status: "failed",
        prompt: "请计算题目",
        expected: "42",
        answer: "用于检查长回复的内容。".repeat(100),
        request_id: `request-${index}`,
      })),
    };
    const runningReport = {
      ...report,
      planned_probes: 74,
      probes: report.probes,
      current_probes: Array.from({ length: 4 }, (_, index) => ({
        id: `reference-${index}`,
        label: `知识边界 · chinese_literature · ${index + 1}`,
        category: "public_reference",
        status: "running",
        round: 1,
        started_at: new Date().toISOString(),
      })),
      current_probe: {
        id: "reference-0",
        label: "知识边界 · chinese_literature · 1",
        category: "public_reference",
        status: "running",
        started_at: new Date().toISOString(),
      },
      probe_plan: [
        ...report.probes,
        ...Array.from({ length: 67 }, (_, index) => ({
          id: `reference-${index}`,
          label: `知识边界 · chinese_literature · ${index + 1}`,
          category: "public_reference",
          round: 1,
        })),
      ],
    };
    const task = (id: string) => ({
      id,
      skill: "onboarding",
      operation: "onboarding-probe",
      status: id === "probe" && !finished ? "running" : "succeeded",
      progress: finished ? 100 : 39,
      message: "正在检测",
      created_at: "",
      updated_at: "",
      result:
        id !== "probe"
          ? { models: ["default-model", model], steps: [] }
          : {
              steps: [
                {
                  stage: "request",
                  status: finished || finishing ? "succeeded" : "running",
                  started_at: "2026-10-16T00:00:00Z",
                },
              ],
              total: 1,
              completed: finished ? 1 : 0,
              phase: "testing",
              ...(finished ? { tests: [report] } : { active: { verification: runningReport } }),
              probe_result: {
                status: "completed",
                message: "请查看逐项结果",
                request_model: model,
                actual_model: model,
                latency_ms: 100,
                http_status: 0,
              },
            },
    });
    const upstream = {
      upstream_id: "u1",
      host: "upstream.test",
      name: "测试上游",
      base_url: "https://upstream.test",
      account_base_url: "https://upstream.test",
      upstream_type: "sub2api",
      auth_mode: "sub2api_user_token",
      recharge_rate: "1",
      balance: "10",
      headers: {},
      header_names: [],
      cookie_names: [],
      groups: [],
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
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
        "/api/overview": {
          database_available: true,
          account_count: 0,
          group_count: 1,
          open_alerts: 0,
          recent_runs: 0,
          last_activity: null,
          mode: "完全模式",
        },
        "/api/model-checks/capabilities": {
          public_references: [{ model, aliases: [model], requests: 12, methods: ["tokenizer"] }],
        },
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "测试" },
        "/api/upstreams": { hosts: [] },
        "/api/catalog/upstreams": [],
        "/api/upstreams/upstream.test/configuration": upstream,
        "/api/onboarding/prepare": {
          upstream,
          candidates: [
            {
              number: 1,
              upstream_id: "u1",
              host: upstream.host,
              upstream_name: upstream.name,
              group_id: "6",
              group_name: "测试分组",
              platform: "openai",
              status: "active",
              multiplier: "1",
              can_create_key: true,
              can_bind_existing_key: false,
              bindable: true,
              bound: false,
              key_present: false,
              bound_accounts: [],
              unavailable_reason: null,
            },
          ],
        },
        "/api/catalog/groups": [
          {
            id: "1",
            name: "测试分组",
            platform: "openai",
            platforms: ["openai"],
            account_count: 0,
          },
        ],
        "/api/dictionaries": dictionaryResponse(route.request().url(), [
          { value: "1", name: "测试分组" },
        ]),
        "/api/config/account-settings": {
          default: {},
          groups: [],
          platform_probe_models: { openai: "default-model" },
        },
      };
      if (path.includes("/onboarding/probe/tasks/")) {
        const action = path.split("/").at(-1)!;
        if (action === "probe")
          expect(route.request().postDataJSON()).toMatchObject({ model, mode: detectionMode });
        await route.fulfill({ json: task(action) });
      } else if (path.startsWith("/api/tasks/"))
        await route.fulfill({ json: task(path.split("/").at(-1)!) });
      else if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": test\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else {
        unhandled.push(path);
        await route.fulfill({ status: 503, json: { detail: `测试未配置 ${path}` } });
      }
    });
    await page.goto("/onboarding?host=upstream.test&upstream_type=sub2api&group_id=%226%22");
    await page.getByRole("button", { name: "探活测试", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "测试账号连接" });
    await expect(dialog.getByRole("button", { name: "开始测试" })).toBeEnabled();
    const selector = dialog.getByRole("combobox", { name: "选择测试模型" });
    await selector.click();
    await page.getByRole("option", { name: model, exact: true }).click();
    await dialog.getByRole("combobox", { name: "选择测试模式" }).click();
    await page.getByRole("option", { name: "模型检测", exact: true }).click();
    await expect(selector).toContainText(model);
    const modeSelector = dialog.getByRole("combobox", { name: "选择测试模式" });
    for (const mode of ["常规请求", "流式请求", "糖果题测试", "模型检测"]) {
      await modeSelector.click();
      await page.getByRole("option", { name: mode, exact: true }).click();
      // Read both controls in the same frame while the dialog changes size.
      await expect
        .poll(() =>
          dialog.getByRole("group", { name: "探活参数" }).evaluate((node) => {
            const modelBox = node
              .querySelector('[aria-label="选择测试模型"]')!
              .getBoundingClientRect();
            const modeBox = node
              .querySelector('[aria-label="选择测试模式"]')!
              .getBoundingClientRect();
            return Math.abs(modelBox.y - modeBox.y) < 2 && modeBox.x > modelBox.x;
          }),
        )
        .toBe(true);
      await expect(selector).toContainText(model);
    }
    await expect(dialog.getByText(/额外 12 次请求/)).toBeVisible();
    await modeSelector.click();
    await expect(page.getByRole("option", { name: "行为指纹", exact: true })).toHaveCount(0);
    await expect(page.getByRole("option", { name: "真伪检测", exact: true })).toHaveCount(0);
    await page.getByRole("option", { name: "模型检测", exact: true }).click();
    await dialog.getByRole("button", { name: "开始测试" }).click();
    await expect(dialog.getByRole("region", { name: "逐项检测过程" })).toBeVisible();
    await expect(dialog.getByRole("progressbar")).toHaveCount(0);
    await expect(dialog.getByRole("button", { name: "取消任务", exact: true })).toHaveCount(0);
    await expect(selector).toBeDisabled();
    await expect(dialog.getByRole("button", { name: "取消并关闭", exact: true })).toBeInViewport();
    const process = dialog.getByRole("region", { name: "逐项检测过程" });
    await expect(process).toBeVisible();
    expect(await process.evaluate((node) => getComputedStyle(node).overflowY)).toBe("visible");
    const group = process.getByRole("region", { name: "公开参考对比" });
    const toggle = group.getByRole("button", { name: /公开参考对比/ });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await expect(process.getByRole("region", { name: "当前检测" })).toBeVisible();
    await expect(
      process.getByRole("region", { name: "当前检测" }).getByText("等待回复", { exact: true }),
    ).toHaveCount(4);
    await expect(process.getByText("当前检测 · 4 项进行中")).toBeVisible();
    await page.screenshot({ path: test.info().outputPath("compact-verification-running.png") });
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(
      group.getByText("知识边界 · chinese_literature · 67", { exact: true }).first(),
    ).toBeVisible();
    const body = dialog.locator('[data-slot="dialog-body"]');
    expect(await body.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
    expect(await body.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    await body.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    await expect(dialog.getByRole("button", { name: "取消并关闭", exact: true })).toBeInViewport();
    await toggle.click();
    await body.evaluate((node) => {
      node.scrollTop = 0;
    });
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    finishing = true;
    await expect(dialog.getByRole("button", { name: "正在收尾", exact: true })).toBeDisabled();
    finished = true;
    await expect(dialog.getByRole("region", { name: "账号模型检测结果" })).toBeVisible();
    await expect(dialog.getByRole("button", { name: "重试", exact: true })).toBeInViewport();
    await expect(dialog.getByRole("button", { name: "重试", exact: true })).toBeEnabled();
    await expect(selector).toBeEnabled();
    await expect(dialog.getByText("检测完成 · 1 个组合")).toBeVisible();
    await expect(dialog.getByText("检测失败 · 1 个组合")).toHaveCount(0);
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    expect(unhandled).toEqual([]);
    await expect(page.getByText(/测试未配置/)).toHaveCount(0);
    await page.screenshot({ path: test.info().outputPath("inline-verification-result.png") });
    await expect(dialog.getByRole("button", { name: "查看检测证据" })).toHaveCount(0);
    await dialog.getByRole("button", { name: "来源与指纹" }).click();
    await expect(dialog.getByRole("region", { name: "公开模型指纹对比" })).toBeVisible();
    await expect(
      dialog.getByRole("region", { name: "公开模型指纹对比" }).getByText("公开参考：无法判定"),
    ).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(1);
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    await page.screenshot({ path: test.info().outputPath("onboarding-verification.png") });
    await dialog.getByRole("button", { name: "逐项检测结果" }).click();
    await expect(dialog.getByRole("button", { name: "重试", exact: true })).toBeEnabled();
  });
}
