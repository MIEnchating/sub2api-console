import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("探活真实阶段在桌面和手机上可见，创建 Key、请求和清理完成前不会提前标记成功", async ({
  page,
}) => {
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
  const candidate = {
    number: 1,
    upstream_id: "u1",
    host: upstream.host,
    upstream_name: upstream.name,
    group_id: "6",
    group_name: "Kimi",
    platform: "kimi",
    status: "active",
    multiplier: "0.25",
    can_create_key: true,
    can_bind_existing_key: false,
    bindable: true,
    bound: false,
    key_present: false,
    bound_accounts: [],
    unavailable_reason: null,
  };
  let phase = "create";
  const step = (stage: string, status: string) => ({
    stage,
    status,
    started_at: "2026-09-11T00:00:00Z",
    finished_at: status === "running" ? undefined : "2026-09-11T00:00:01Z",
  });
  const task = (id: string) => {
    let status = "running";
    let steps = [step("create_key", "running")];
    if (id === "models" && phase !== "create") {
      status = "succeeded";
      steps = [step("create_key", "succeeded"), step("models", "succeeded")];
    }
    if (id === "probe") {
      steps = [
        step("reuse_key", "succeeded"),
        step("request", phase === "request" ? "running" : "succeeded"),
      ];
      if (phase !== "request")
        steps.push(step("cleanup_key", phase === "finished" ? "succeeded" : "running"));
      if (phase === "finished") status = "succeeded";
    }
    if (id === "cleanup") {
      status = "succeeded";
      steps = [step("cleanup_key", "skipped")];
    }
    return {
      id,
      status,
      skill: "onboarding",
      operation: `onboarding-probe-${id}`,
      progress: 0,
      message: "操作完成",
      created_at: "",
      updated_at: "",
      result: {
        steps,
        models: ["kimi-k2"],
        probe_result: {
          status: "passed",
          message: "请求成功",
          request_model: "kimi-k2",
          actual_model: "kimi-k2",
          response_text: "你好",
          latency_ms: 50,
          http_status: 200,
        },
      },
    };
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/upstreams": { hosts: [] },
      "/api/upstreams/upstream.test/configuration": upstream,
      "/api/onboarding/prepare": { upstream, candidates: [candidate] },
      "/api/groups": [
        {
          id: "1",
          name: "国产平价",
          platform: "composite",
          account_count: 0,
          platforms: ["composite"],
        },
      ],
      "/api/config/account-settings": {
        default: {},
        groups: [],
        platform_probe_models: { kimi: "kimi-k2" },
      },
    };
    if (path.includes("/onboarding/probe/tasks/")) {
      const id = path.split("/").at(-1)!;
      if (id === "probe") phase = "request";
      await route.fulfill({ json: { ...task(id), status: "queued", result: { steps: [] } } });
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
  const timeline = dialog.getByRole("list", { name: "探活过程" });
  const modelRow = dialog.getByRole("group", { name: "测试模型选择与获取" });
  const selector = modelRow.getByRole("combobox");
  const refresh = modelRow.getByRole("button");
  const selectorBox = await selector.boundingBox();
  const refreshBox = await refresh.boundingBox();
  expect(selectorBox).not.toBeNull();
  expect(refreshBox).not.toBeNull();
  expect(Math.abs(selectorBox!.y - refreshBox!.y)).toBeLessThanOrEqual(4);
  expect(refreshBox!.x).toBeGreaterThan(selectorBox!.x);
  await expect(dialog.getByRole("region", { name: "测试模型响应" })).toBeVisible();
  await expect(dialog.getByText("点击“开始测试”查看响应")).toBeVisible();
  await expect(timeline).toHaveCount(0);
  const expand = dialog.getByRole("button", { name: /创建临时上游 Key/ });
  await expect(expand).toHaveAttribute("aria-expanded", "false");
  await expand.click();
  await expect(timeline.getByText("创建临时上游 Key")).toBeVisible();
  await expect(dialog.getByRole("button", { name: "开始测试" })).toBeDisabled();
  await expect(dialog.getByRole("progressbar")).toHaveCount(0);
  await page.screenshot({ path: test.info().outputPath("probe-create-key.png") });
  phase = "ready";
  await expect(dialog.getByRole("button", { name: "开始测试" })).toBeEnabled();
  await dialog.getByRole("button", { name: "开始测试" }).click();
  await expect(timeline.getByText("发送探活请求并等待响应")).toBeVisible();
  phase = "cleanup";
  await expect(timeline.getByText("清理临时上游 Key")).toBeVisible();
  await expect(dialog.getByText("测试完成！")).toHaveCount(0);
  phase = "finished";
  await expect(dialog.getByText("测试完成！")).toBeVisible();
  await dialog.getByRole("button", { name: /清理临时上游 Key.*已完成/ }).click();
  await expect(timeline).toHaveCount(0);
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "重试", exact: true })).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("probe-completed.png") });
});
