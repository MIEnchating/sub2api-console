import { expect, test } from "@playwright/test";
import { template } from "../../src/features/account-workbench/__tests__/template-fixture";
import { pageFixtures } from "./fixtures/page-shell";

const preview = {
  ...template,
  name: "team-5x-通用模板-旗舰",
  source_name: "template-owner@example.test",
  config: {
    ...template.config,
    concurrency: 20,
    priority: 1,
    rate_multiplier: "1",
    load_factor: "100",
    credential_extras: {
      plan_type: "self_serve_business_prolite",
      model_mapping: {
        "codex-auto-review": "codex-auto-review",
        "gpt-5.5": "gpt-5.5",
        "gpt-5.6-luna": "gpt-5.6-luna",
        "gpt-5.6-sol": "gpt-5.6-sol",
        "gpt-5.6-terra": "gpt-5.6-terra",
        "gpt-6-astra": "gpt-6-astra",
        "gpt-reserve": "gpt-reserve",
      },
    },
  },
};

test.beforeEach(async ({ page }, info) => {
  await page.addInitScript(
    (theme) => {
      localStorage.setItem("sub2api-console-theme", theme);
    },
    info.project.name === "desktop-light" ? "light" : "dark",
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局验收" },
      "/api/dictionaries": { items: [] },
      "/api/account-workbench/templates": {
        revision: 1,
        preferred_id: preview.id,
        items: [preview],
      },
      "/api/account-workbench/accounts": [template.summary],
      "/api/account-workbench/template-source/41": preview,
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "配置模板", exact: true }).click();
});

test("单模板铺满列表，映射在宽卡片中分栏、窄卡片中独占整行", async ({ page }, info) => {
  const card = page.getByRole("article", { name: preview.name });
  const mapping = card.getByRole("table", { name: "模型映射" });
  await expect(mapping.getByRole("row")).toHaveCount(4);
  const cardBox = await card.boundingBox();
  const mappingBox = await mapping.boundingBox();
  const listBox = await page.getByLabel("模板列表", { exact: true }).boundingBox();
  expect(cardBox!.width).toBeGreaterThan(listBox!.width * 0.95);
  expect(mappingBox!.width).toBeGreaterThan(
    cardBox!.width * (info.project.name === "desktop-light" ? 0.4 : 0.8),
  );
  const metricsBox = await card.getByText("并发", { exact: true }).boundingBox();
  if (info.project.name === "desktop-light") {
    expect(mappingBox!.x).toBeGreaterThan(cardBox!.x + cardBox!.width / 2);
  } else {
    expect(mappingBox!.y).toBeGreaterThan(metricsBox!.y + metricsBox!.height);
  }
  const toggle = card.getByRole("button", { name: /展开全部 7 条映射|收起映射/ });
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(mapping.getByRole("row")).toHaveCount(8);
  expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await toggle.click();
  await expect(card.getByRole("button", { name: "设为当前" })).toBeDisabled();
  await page.screenshot({ path: test.info().outputPath("模板列表.png"), fullPage: true });
});

test("低高度窗口中预览只滚动内容，标题和保存取消按钮始终可见", async ({ page }, info) => {
  await page.setViewportSize({
    width: info.project.name === "desktop-light" ? 1280 : 390,
    height: 600,
  });
  await page.getByRole("button", { name: "重新同步" }).click();
  const dialog = page.getByRole("dialog", { name: "重新同步模板" });
  const save = dialog.getByRole("button", { name: "保存模板" });
  const cancel = dialog.getByRole("button", { name: "取消" });
  await expect(save).toBeEnabled();
  await expect(save).toBeInViewport();
  await expect(cancel).toBeInViewport();
  const body = dialog.locator('[data-slot="dialog-body"]');
  expect(await body.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true);
  await body.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(
    dialog.getByRole("cell", { name: "gpt-reserve", exact: true }).first(),
  ).toBeInViewport();
  await expect(dialog.getByRole("heading", { name: "重新同步模板" })).toBeInViewport();
  await expect(save).toBeInViewport();
  await expect(cancel).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("配置预览.png"), fullPage: true });
  await cancel.click();
  await expect(dialog).toHaveCount(0);
});

test("超长模型名、来源账号与分组名称换行显示且不撑宽卡片和弹窗", async ({ page }) => {
  const longName = "long-model-name-".repeat(20);
  const longTemplate = {
    ...preview,
    source_name: "long-account-name-".repeat(20),
    summary: { ...preview.summary, groups: [{ id: "7", name: "long-group-name-".repeat(20) }] },
    config: { ...preview.config, credential_extras: { model_mapping: { [longName]: longName } } },
  };
  await page.route("**/api/account-workbench/templates", (route) =>
    route.fulfill({
      json: { revision: 1, preferred_id: preview.id, items: [longTemplate] },
    }),
  );
  await page.route("**/api/account-workbench/template-source/41", (route) =>
    route.fulfill({ json: longTemplate }),
  );
  await page.getByRole("button", { name: "刷新", exact: true }).click();
  const panel = page.getByRole("tabpanel");
  await expect(panel.getByRole("cell", { name: longName }).first()).toBeVisible();
  expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.getByRole("button", { name: "重新同步" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("cell", { name: longName }).first()).toBeAttached();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "保存模板" })).toBeInViewport();
});

test("手动创建模板桌面分区并排、手机顺序排列，低高度时只滚动表单内容", async ({ page }, info) => {
  await page.getByRole("button", { name: "创建模板", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "手动创建模板" });
  const settings = dialog.getByRole("group", { name: "调度与计费" });
  const mapping = dialog.getByRole("region", { name: "模型映射设置" });
  await expect(mapping.getByRole("textbox", { name: "模型映射" })).toBeVisible();
  const settingsBox = await settings.boundingBox();
  const mappingBox = await mapping.boundingBox();
  if (info.project.name === "desktop-light") {
    expect(mappingBox!.x).toBeGreaterThan(settingsBox!.x + settingsBox!.width);
  } else {
    expect(mappingBox!.y).toBeGreaterThan(settingsBox!.y + settingsBox!.height);
  }
  const name = dialog.getByRole("textbox", { name: "模板名称" });
  await name.fill("自定义导入模板");
  await expect(name).toHaveValue("自定义导入模板");
  await page.screenshot({ path: test.info().outputPath("创建配置模板.png"), fullPage: true });
  await page.setViewportSize({
    width: info.project.name === "desktop-light" ? 1280 : 390,
    height: 600,
  });
  const save = dialog.getByRole("button", { name: "保存模板" });
  const cancel = dialog.getByRole("button", { name: "取消" });
  await expect(save).toBeInViewport();
  await expect(cancel).toBeInViewport();
  const body = dialog.locator('[data-slot="dialog-body"]');
  expect(await body.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true);
  await body.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(dialog.getByRole("textbox", { name: "备注", exact: true })).toBeInViewport();
  await expect(dialog.getByRole("heading", { name: "手动创建模板" })).toBeInViewport();
  await expect(save).toBeInViewport();
  await expect(cancel).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await cancel.click();
  await expect(dialog).toHaveCount(0);
});
