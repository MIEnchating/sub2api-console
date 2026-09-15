import { expect, test, type Page } from "@playwright/test";

import type { PricingConfig, PricingSnapshot } from "../../src/api";
import { pricing } from "./fixtures/settings";

async function setupPricing(
  page: Page,
  theme: string | null,
  initialSnapshot: PricingSnapshot = pricing,
): Promise<PricingConfig[]> {
  let snapshot = initialSnapshot;
  const saves: PricingConfig[] = [];
  await page.addInitScript(
    (value) => localStorage.setItem("sub2api-console-theme", value ?? "light"),
    theme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/pricing/config" && route.request().method() === "PUT") {
      const config = route.request().postDataJSON() as PricingConfig;
      saves.push(config);
      snapshot = { ...snapshot, config };
      await route.fulfill({ json: snapshot });
      return;
    }
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价格布局测试" },
      "/api/pricing": snapshot,
      "/api/dictionaries": { items: [] },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  return saves;
}

test("超长规则和分组名称完整布局，操作保留首行且售价与稳定 ID 位于名称下方", async ({
  page,
  colorScheme,
}) => {
  const longName = "生产环境专用分组OpenAI".repeat(12);
  await setupPricing(page, colorScheme, {
    ...pricing,
    config: {
      ...pricing.config,
      exchange_group_set_names: ["生产环境的长期运行价格规则".repeat(4)],
    },
    groups: [
      { ...pricing.groups[0], name: longName },
      pricing.groups[2],
      pricing.groups[1],
      pricing.groups[3],
    ],
  });
  await page.goto("/pricing-config");
  const rule = page.getByRole("region", { name: "规则 1", exact: true });
  const input = rule.getByRole("textbox", { name: "互换组 1 规则名称" });
  const collapse = rule.getByRole("button", { name: "收起互换组 1" });
  const remove = rule.getByRole("button", { name: "删除互换组 1" });
  await expect(input).toBeVisible();
  const inputBounds = (await input.boundingBox())!;
  const collapseBounds = (await collapse.boundingBox())!;
  const removeBounds = (await remove.boundingBox())!;
  expect(collapseBounds.x).toBeGreaterThanOrEqual(inputBounds.x + inputBounds.width);
  expect(removeBounds.x).toBeGreaterThanOrEqual(collapseBounds.x + collapseBounds.width);
  expect(collapseBounds.y).toBe(inputBounds.y);
  expect(removeBounds.y).toBe(inputBounds.y);
  for (const button of [collapse, remove]) {
    await expect(button).toHaveCSS("width", "32px");
    await expect(button).toHaveCSS("height", "32px");
  }
  const headerBounds = (await rule.locator('[data-slot="exchange-set-heading"]').boundingBox())!;
  const summaryBounds = (await rule.locator('[data-slot="exchange-set-summary"]').boundingBox())!;
  expect(summaryBounds.y).toBeGreaterThanOrEqual(headerBounds.y + headerBounds.height);
  const selected = rule.locator('[data-slot="exchange-group-card"]').filter({
    has: page.getByRole("checkbox", { name: `互换组 1 分组 ${longName}` }),
  });
  const name = selected.getByText(longName, { exact: true });
  const metadata = selected.locator('[data-slot="exchange-group-metadata"]');
  await expect(name).toBeVisible();
  await expect(metadata).toContainText("售价 0.2");
  await expect(metadata).toContainText("#6");
  const nameBounds = (await name.boundingBox())!;
  const metadataBounds = (await metadata.boundingBox())!;
  expect(metadataBounds.y).toBeGreaterThanOrEqual(nameBounds.y + nameBounds.height);
  expect(
    await name.evaluate(
      (element) =>
        element.scrollWidth <= element.clientWidth && element.scrollHeight <= element.clientHeight,
    ),
  ).toBe(true);
  const unselected = rule.locator('[data-slot="exchange-group-card"]').filter({
    has: page.getByRole("checkbox", { name: "互换组 1 分组 codex-pro" }),
  });
  const unselectedBounds = (await unselected.boundingBox())!;
  const optionBounds = (await unselected
    .locator('[data-slot="exchange-group-option"]')
    .boundingBox())!;
  expect(unselectedBounds.height).toBeGreaterThan(optionBounds.height);
  for (const element of [rule, page.locator('[data-slot="page-content"]')]) {
    expect(await element.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  }
  await page.screenshot({ path: test.info().outputPath("pricing-config-long-names.png") });
});

test("键盘折叠与展开互换组后仍保留规则名称、分组选择和倍率并保存相同配置", async ({
  page,
  colorScheme,
}) => {
  const saves = await setupPricing(page, colorScheme);
  await page.goto("/pricing-config");
  const name = page.getByRole("textbox", { name: "互换组 1 规则名称" });
  await name.fill("生产渠道价格");
  const minimum = page.getByRole("textbox", { name: "分组 codex-平价 最低迁入倍率" });
  await minimum.fill("0.10000000000000001");
  const option = page.getByRole("checkbox", { name: "互换组 1 分组 codex-pro" });
  await option.focus();
  await page.keyboard.press("Space");
  await expect(option).toBeChecked();
  const collapse = page.getByRole("button", { name: "收起互换组 1" });
  await collapse.focus();
  await page.keyboard.press("Enter");
  const expand = page.getByRole("button", { name: "展开互换组 1" });
  await expect(expand).toHaveAttribute("aria-expanded", "false");
  await expect(expand).toBeFocused();
  await expect(option).toBeHidden();
  await expect(name).toHaveValue("生产渠道价格");
  await page.keyboard.press("Space");
  await expect(collapse).toHaveAttribute("aria-expanded", "true");
  await expect(option).toBeChecked();
  await expect(minimum).toHaveValue("0.10000000000000001");
  await page.getByRole("button", { name: "保存配置", exact: true }).click();
  await expect.poll(() => saves.length).toBe(1);
  expect(saves[0]).toMatchObject({
    exchange_group_set_names: ["生产渠道价格"],
    exchange_group_sets: [["6", "7", "8"]],
    group_min_cost_multipliers: { "6": "0.10000000000000001" },
  });
  await expect(page.getByRole("button", { name: "保存配置", exact: true })).toBeEnabled();
  await expect(name).toHaveValue("生产渠道价格");
  await expect(minimum).toHaveValue("0.10000000000000001");
});

test("没有可选分组时显示刷新提示，刷新成功后可以继续选择分组", async ({ page, colorScheme }) => {
  const emptySnapshot: PricingSnapshot = {
    ...pricing,
    config: {
      ...pricing.config,
      exchange_group_sets: [[]],
      exchange_group_set_names: ["待设置渠道"],
    },
    groups: [],
  };
  await setupPricing(page, colorScheme, emptySnapshot);
  let reads = 0;
  await page.route("**/api/pricing", (route) => {
    reads += 1;
    return route.fulfill({
      json: reads === 1 ? emptySnapshot : { ...emptySnapshot, groups: pricing.groups },
    });
  });
  await page.goto("/pricing-config");
  const choices = page.getByRole("group", { name: "互换组 1 可选分组" });
  await expect(choices.getByText("暂无可选分组", { exact: true })).toBeVisible();
  await expect(choices.getByText(/刷新/)).toBeVisible();
  await expect(choices.getByRole("checkbox")).toHaveCount(0);
  await page.getByRole("button", { name: "刷新价格数据", exact: true }).click();
  const option = choices.getByRole("checkbox", { name: "互换组 1 分组 codex-平价" });
  await expect(option).toBeEnabled();
  await expect(choices.getByText("暂无可选分组", { exact: true })).toHaveCount(0);
  await option.check();
  await expect(option).toBeChecked();
});
