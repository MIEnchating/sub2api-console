import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

const upstream = {
  upstream_id: "layout",
  host: "layout.example.test",
  name: "测试上游",
  base_url: "https://layout.example.test",
  account_base_url: "https://layout.example.test",
  upstream_type: "sub2api",
  auth_mode: "sub2api_user_token",
  recharge_rate: "1",
  balance: "10",
  groups: [],
  headers: {},
  header_names: [],
  cookie_names: [],
};
const model = "gm-deepseek-v4.1-flash";
test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const task = {
      id: "layout-models",
      status: "succeeded",
      skill: "onboarding",
      operation: "model-options",
      progress: 100,
      message: "",
      result: { models: [model, "gpt-5.2"] },
      created_at: "",
      updated_at: "",
    };
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/policy": {
        available: true,
        probe_model: "global-probe",
        global_strategy: "balanced",
        advanced_policy: {},
        group_strategies: [],
        configuration_errors: [],
      },
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/dictionaries": { items: [] },
      "/api/upstreams/layout.example.test/configuration": upstream,
      "/api/groups": [{ id: "3", name: "国产-平价", platform: "openai", account_count: 0 }],
      "/api/onboarding/prepare": {
        upstream,
        candidates: [
          {
            number: 1,
            upstream_id: "layout",
            host: upstream.host,
            upstream_name: upstream.name,
            group_id: "7",
            group_name: "特价国模",
            platform: "openai",
            status: "active",
            multiplier: "1",
            bindable: true,
            can_create_key: true,
            can_bind_existing_key: false,
            bound: false,
            key_present: false,
            bound_accounts: [],
            unavailable_reason: null,
          },
        ],
      },
      "/api/onboarding/concurrency-preview": { items: [{ concurrency: 100 }] },
      "/api/onboarding/probe/tasks/model-options": task,
      "/api/tasks/layout-models": task,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

for (const viewport of [
  { width: 1440, height: 900 },
  { width: 390, height: 664 },
  { width: 320, height: 568 },
]) {
  test(`${viewport.width}px 确认弹窗映射对齐且内容滚动时提交按钮保持可见`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const errors: Error[] = [];
    page.on("pageerror", (error) => errors.push(error));
    await page.goto("/onboarding?host=layout.example.test&upstream_type=sub2api&group_id=%227%22");
    const groups = page.getByRole("combobox", { name: "特价国模 本地分组" });
    await groups.click();
    await page.getByRole("option", { name: /国产-平价/ }).click();
    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: "预览添加账号" }).click();
    const dialog = page.getByRole("dialog", { name: "确认账号绑定变更" });
    await dialog.getByRole("button", { name: "获取模型" }).click();
    await expect(dialog.getByText("已获取 2 个上游模型")).toBeVisible();
    await expect(dialog.getByRole("note", { name: "继承的探活配置" })).toContainText(
      "继承全局模型：global-probe",
    );
    const probe = dialog.getByRole("combobox", { name: "特价国模 → 国产-平价 探活模型" });
    await probe.scrollIntoViewIfNeeded();
    const selectHeight = (await probe.boundingBox())!.height;
    expect(selectHeight).toBe(32);
    await dialog.getByRole("button", { name: "手动填写" }).click();
    const manual = dialog.getByRole("textbox", { name: "特价国模 → 国产-平价 探活模型" });
    expect((await manual.boundingBox())!.height).toBe(selectHeight);
    await manual.fill(
      Array.from({ length: 5 }, (_, index) => `long-custom-model-${index}-` + "x".repeat(100)).join(
        "\n",
      ),
    );
    const manualHeight = (await manual.boundingBox())!.height;
    expect(manualHeight).toBe(32);
    expect(await manual.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
      true,
    );
    await dialog.getByRole("button", { name: "从列表选择" }).click();
    expect((await probe.boundingBox())!.height).toBe(manualHeight);
    await dialog.getByRole("button", { name: "手动填写" }).click();
    expect((await manual.boundingBox())!.height).toBe(manualHeight);
    await manual.fill("");
    await dialog.getByRole("button", { name: "从列表选择" }).click();
    for (let index = 0; index < 3; index++) {
      await dialog.getByRole("button", { name: "添加模型映射" }).click();
      const row = dialog.getByRole("group", { name: `第 ${index + 1} 条模型映射`, exact: true });
      await row.getByRole("combobox", { name: "上游模型" }).click();
      await page.getByRole("option", { name: model, exact: true }).click();
      await row
        .getByRole("textbox", { name: "请求模型" })
        .fill(index === 0 ? "deepseek-v4.1-flash" : `custom-model-${index}`);
    }
    const row = dialog.getByRole("group", { name: "第 1 条模型映射", exact: true });
    await row.scrollIntoViewIfNeeded();
    const source = await row.getByRole("textbox", { name: "请求模型" }).boundingBox();
    const target = await row.getByRole("combobox", { name: "上游模型" }).boundingBox();
    expect(source).not.toBeNull();
    expect(target).not.toBeNull();
    const bounds = await dialog.boundingBox();
    expect(bounds).not.toBeNull();
    if (viewport.width > 640) {
      expect(bounds!.width).toBeGreaterThanOrEqual(740);
      expect(bounds!.width).toBeLessThanOrEqual(800);
      expect(Math.abs(source!.y - target!.y)).toBeLessThan(2);
    } else {
      expect(target!.y).toBeGreaterThanOrEqual(source!.y + source!.height);
      expect(bounds!.width).toBeLessThanOrEqual(viewport.width - 24);
    }
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await page.screenshot({ path: test.info().outputPath(`confirmation-${viewport.width}.png`) });
    for (let index = 0; index < 8; index++)
      await dialog.getByRole("button", { name: "添加模型映射" }).click();
    await dialog
      .getByRole("group", { name: "第 11 条模型映射", exact: true })
      .scrollIntoViewIfNeeded();
    const body = dialog.locator('[data-slot="dialog-body"]');
    expect(await body.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
      true,
    );
    await expect(dialog.getByRole("button", { name: "确认提交 1 项变更" })).toBeInViewport({
      ratio: 1,
    });
    await expect(dialog.getByRole("button", { name: "取消" })).toBeInViewport({ ratio: 1 });
    expect(errors).toEqual([]);
  });
}
