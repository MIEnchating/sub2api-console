import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { dictionaryResponse } from "./fixtures/dictionaries";

test("动画探活在等待、运行和完成时保持预览尺寸，糖果题和重试保留模型下拉", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  let phase = "ready";
  let mode = "animation";
  let sequence = 0;
  const task = (id: string) => ({
    id,
    skill: "onboarding",
    operation: "onboarding-probe",
    status: phase === "running" && id.startsWith("probe") ? "running" : "succeeded",
    progress: 0,
    message: "完成",
    created_at: "",
    updated_at: "",
    result: {
      models: ["model-a", "model-b"],
      steps: [
        {
          stage: phase === "running" ? "request" : "cleanup_key",
          status: phase === "running" ? "running" : "succeeded",
          started_at: "2026-10-13T00:00:00Z",
        },
      ],
      probe_result: {
        status: "passed",
        message: mode === "candy" ? "糖果题测试通过" : "动画测试完成",
        request_model: "model-b",
        actual_model: "model-b",
        response_text: mode === "candy" ? "21" : "",
        http_status: 0,
        latency_ms: 103727,
        ...(mode === "animation"
          ? {
              animation: {
                account_id: "",
                account_name: "接入前探活",
                model: "model-b",
                request_id: "animation",
                status: "succeeded",
                duration_ms: 103727,
                completed_at: "2026-10-13T00:00:00Z",
                html: '<!DOCTYPE html><html><body style="background:#e7eee8"><h1>动画预览</h1><svg viewBox="0 0 800 450"><circle cx="400" cy="220" r="100" fill="#658870"/></svg></body></html>',
              },
            }
          : {}),
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
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/upstreams": { hosts: [] },
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
          name: "测试 OpenAI",
          platform: "openai",
          platforms: ["openai"],
          account_count: 0,
        },
      ],
      "/api/dictionaries": dictionaryResponse(route.request().url(), [
        { value: "1", name: "测试 OpenAI" },
      ]),
      "/api/config/account-settings": {
        default: {},
        groups: [],
        platform_probe_models: { openai: "model-a" },
      },
    };
    if (path.includes("/onboarding/probe/tasks/")) {
      const action = path.split("/").at(-1)!;
      if (action === "probe") {
        const body = route.request().postDataJSON();
        expect(body.model).toBe("model-b");
        mode = body.mode;
        phase = "running";
      }
      await route.fulfill({ json: task(`${action}-${++sequence}`) });
    } else if (path.startsWith("/api/tasks/"))
      await route.fulfill({ json: task(path.split("/").at(-1)!) });
    else if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": test\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: `测试未配置 ${path}` } });
  });
  await page.goto("/onboarding?host=upstream.test&upstream_type=sub2api&group_id=%226%22");
  await page.getByRole("button", { name: "探活测试", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "测试账号连接" });
  const model = dialog.getByRole("combobox", { name: "选择测试模型" });
  await expect(dialog.getByRole("button", { name: "开始测试" })).toBeEnabled();
  await model.click();
  await page.getByRole("option", { name: "model-b", exact: true }).click();
  await dialog.getByRole("combobox", { name: "选择测试模式" }).click();
  await page.getByRole("option", { name: "动画测试", exact: true }).click();
  await expect(model).toContainText("model-b");
  expect(await model.evaluate((node) => node.tagName)).toBe("BUTTON");
  const stage = dialog.getByRole("region", { name: "动画展示区域" });
  const stageHeight = await stage.evaluate((node) => node.clientHeight);
  const stageBox = await stage.boundingBox();
  expect(Math.abs(stageBox!.width / stageBox!.height - 4 / 3)).toBeLessThan(0.02);
  const body = dialog.locator('[data-slot="dialog-body"]');
  expect(await body.evaluate((node) => node.scrollHeight - node.clientHeight)).toBeLessThanOrEqual(
    1,
  );

  const modelBox = await model.boundingBox();
  const modeBox = await dialog.getByRole("combobox", { name: "选择测试模式" }).boundingBox();
  if (page.viewportSize()!.width >= 640) expect(Math.abs(modelBox!.y - modeBox!.y)).toBeLessThan(2);
  else expect(modeBox!.y).toBeGreaterThan(modelBox!.y);
  await dialog.getByRole("button", { name: "开始测试" }).click();
  await expect(dialog.getByRole("status", { name: "正在生成动画，请稍候" })).toBeVisible();
  await expect(model).toBeDisabled();
  expect(await stage.evaluate((node) => node.clientHeight)).toBe(stageHeight);
  await expect(dialog.getByRole("button", { name: "取消并关闭", exact: true })).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("animation-running.png") });
  phase = "finished";
  const preview = dialog.getByRole("region", { name: "动画测试预览" });
  await expect(preview).toBeVisible();
  await expect(
    preview.locator("iframe").contentFrame().getByRole("heading", { name: "动画预览" }),
  ).toBeVisible();
  expect(await preview.evaluate((node) => node.clientHeight)).toBe(stageHeight);
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "重试", exact: true })).toBeInViewport();
  await expect(dialog.getByRole("heading", { name: "测试账号连接" })).toBeInViewport();

  await expect(dialog.getByText("动画已生成", { exact: true })).toBeVisible();
  await expect(dialog.getByText("103.7 秒", { exact: true })).toBeVisible();
  await expect(dialog.getByText(/HTTP/)).toHaveCount(0);
  expect(await body.evaluate((node) => node.scrollHeight - node.clientHeight)).toBeLessThanOrEqual(
    1,
  );
  await expect(preview).toBeInViewport({ ratio: 1 });
  const process = dialog.getByRole("button", { name: "查看测试过程" });
  await process.click();
  await expect(dialog.getByRole("list", { name: "探活过程" })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "重试", exact: true })).toBeInViewport();
  await process.click();
  await page.screenshot({ path: test.info().outputPath("animation-completed.png") });
  await dialog.getByRole("button", { name: "重试", exact: true }).click();
  await expect(dialog.getByRole("status", { name: "正在生成动画，请稍候" })).toBeVisible();
  phase = "finished";
  await expect(preview).toBeVisible();
  await dialog.getByRole("combobox", { name: "选择测试模式" }).click();
  await page.getByRole("option", { name: "糖果题测试", exact: true }).click();
  await expect(model).toContainText("model-b");
  await dialog.getByRole("button", { name: "开始测试" }).click();
  await expect(dialog.getByRole("button", { name: "测试中", exact: true })).toBeDisabled();
  phase = "finished";
  await expect(dialog.getByText("21", { exact: true })).toBeVisible();
});
