import { expect, test } from "@playwright/test";

import type { PricingConfig } from "../../src/api";
import { pricing } from "./fixtures/settings";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  let config = { ...pricing.config };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/pricing/config" && route.request().method() === "PUT") {
      config = route.request().postDataJSON() as PricingConfig;
      await route.fulfill({ json: { ...pricing, config } });
      return;
    }
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "价格设置测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/pricing": { ...pricing, config },
      "/api/pricing/backups": [],
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("最低迁入倍率可精确保存和清空，编辑不改变已选分组", async ({ page }) => {
  await page.goto("/pricing-config");
  const input = page.getByRole("textbox", { name: "分组 codex-平价 最低迁入倍率", exact: true });
  const selectedGroup = page.getByRole("checkbox", {
    name: /互换组 1 分组 codex-平价/,
  });
  await expect(input).toHaveValue("");
  await input.fill("0.10000000000000000001");
  await page.keyboard.press("Tab");
  await expect(selectedGroup).toBeChecked();
  const saving = page.waitForRequest(
    (request) => request.url().endsWith("/api/pricing/config") && request.method() === "PUT",
  );
  await page.getByRole("button", { name: "保存配置", exact: true }).click();
  const saved = (await saving).postDataJSON() as PricingConfig;
  expect(saved.group_min_cost_multipliers).toEqual({ "6": "0.10000000000000000001" });
  await expect(page.getByText("价格配置已保存，自动调整保持关闭", { exact: true })).toBeVisible();
  await page.reload();
  await expect(input).toHaveValue("0.10000000000000000001");
  await input.clear();
  const clearing = page.waitForRequest(
    (request) => request.url().endsWith("/api/pricing/config") && request.method() === "PUT",
  );
  await page.getByRole("button", { name: "保存配置", exact: true }).click();
  const cleared = (await clearing).postDataJSON() as PricingConfig;
  expect(cleared.group_min_cost_multipliers ?? {}).toEqual({});
  await expect(selectedGroup).toBeChecked();
});

test("超长分组名与最低迁入倍率在窄屏不溢出，非法输入显示字段校验", async ({ page }) => {
  const name = "长名称价格分组".repeat(12);
  await page.route("**/api/pricing", (route) =>
    route.fulfill({
      json: {
        ...pricing,
        groups: pricing.groups.map((group) => (group.id === "6" ? { ...group, name } : group)),
      },
    }),
  );
  await page.goto("/pricing-config");
  const input = page.getByRole("textbox", { name: `分组 ${name} 最低迁入倍率`, exact: true });
  await input.fill("-0.1");
  await page.keyboard.press("Tab");
  await expect(input).toHaveAttribute("aria-invalid", "true");
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await input.fill("0.12");
  await page.keyboard.press("Tab");
  await expect(input).not.toHaveAttribute("aria-invalid", "true");
  await input.scrollIntoViewIfNeeded();
  await expect(input).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: test.info().outputPath("pricing-minimum.png") });
});
