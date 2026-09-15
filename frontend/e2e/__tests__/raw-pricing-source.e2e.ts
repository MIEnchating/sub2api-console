import { readFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

const longModel = `model-00-${"long-context-".repeat(12)}`;
const entries = Object.fromEntries(
  Array.from({ length: 25 }, (_, i) => [
    i === 0 ? longModel : `model-${String(i).padStart(2, "0")}`,
    {
      input_cost_per_token: 0.000003,
      output_cost_per_token: 0.000015,
      cache_read_input_token_cost: 0.0000003,
      cache_creation_input_token_cost: 0.00000375,
      litellm_provider: "anthropic",
      mode: "chat",
      max_input_tokens: 200000,
      max_output_tokens: 64000,
      supports_vision: true,
      supports_function_calling: true,
      custom_options: { region: "long-region-".repeat(80) },
    },
  ]),
);
const raw = `${JSON.stringify(entries, null, 2)}\n`;

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价卡测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/management-model-prices": {
        models: [],
        missing_models: [],
        stale: false,
      },
      "/api/newapi/platforms/layout/remote-model-prices/raw": {
        source_url: `https://raw.example.test/${"directory/".repeat(20)}model-prices.json`,
        content: raw,
        fetched_at: "2026-09-14T07:00:00Z",
        size_bytes: Buffer.byteLength(raw),
        sha256: "a".repeat(64),
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/newapi/prices");
  await page.getByRole("button", { name: "查看原始价卡", exact: true }).click();
});

test("长模型价卡在桌面和窄屏内分页，明细返回后保持焦点且关闭返回原按钮", async ({ page }) => {
  const dialog = page.getByRole("dialog", { name: "远程价卡原始文件" });
  const table = dialog.locator('[data-slot="table-container"]');
  await expect(dialog.getByRole("button", { name: "转到下一页" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("textbox", { name: "搜索模型或厂商" })).toBeInViewport({
    ratio: 1,
  });
  await table.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await dialog.getByRole("button", { name: "转到下一页" }).click();
  await expect(dialog.getByRole("button", { name: "查看 model-24 明细" })).toBeVisible();
  await dialog.getByRole("textbox", { name: "搜索模型或厂商" }).fill("model-00");
  const model = dialog.getByRole("button", { name: `查看 ${longModel} 明细` });
  await expect(model).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: test.info().outputPath("raw-pricing-models.png") });
  await model.click();
  await expect(dialog.getByRole("heading", { name: longModel })).toBeFocused();
  await dialog.getByText("其他字段（1）", { exact: true }).click();
  const details = dialog.getByRole("region", { name: "模型详情" });
  expect(await details.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("raw-pricing-detail.png") });
  await dialog.getByRole("button", { name: "返回模型列表" }).click();
  await expect(model).toBeFocused();
  await expect(dialog.getByRole("textbox", { name: "搜索模型或厂商" })).toHaveValue("model-00");
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "查看原始价卡", exact: true })).toBeFocused();
});

test("低高度窗口中原文独立滚动且文件信息与关闭操作可达", async ({ page, viewport }) => {
  await page.setViewportSize({ width: viewport!.width, height: 480 });
  const dialog = page.getByRole("dialog", { name: "远程价卡原始文件" });
  await dialog.getByRole("tab", { name: "原始 JSON" }).click();
  const content = dialog.getByLabel("原始价卡内容");
  await expect(content).toHaveAttribute("aria-readonly", "true");
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await dialog.getByRole("button", { name: "复制内容" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(raw);
  await dialog
    .locator(".cm-scroller")
    .evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(dialog.getByRole("button", { name: "下载原始文件" })).toBeInViewport({ ratio: 1 });
  await dialog.getByText("文件信息", { exact: true }).click();
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(
    await dialog.evaluate(
      (element) =>
        element.scrollHeight <= element.clientHeight && element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true);
  await page.screenshot({ path: test.info().outputPath("raw-pricing-json.png") });
});

test("下载保留整份原文件，不受模型搜索影响", async ({ page }) => {
  const dialog = page.getByRole("dialog", { name: "远程价卡原始文件" });
  await dialog.getByRole("textbox", { name: "搜索模型或厂商" }).fill("model-24");
  const waiting = page.waitForEvent("download");
  await dialog.getByRole("button", { name: "下载原始文件" }).click();
  const download = await waiting;
  expect(download.suggestedFilename()).toBe("model-prices.json");
  expect(await readFile((await download.path())!, "utf8")).toBe(raw);
});
