import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("动画获取模型使用实际接口交集，失败保留手填值并支持重新获取", async ({ page }) => {
  let failed = false;
  let refreshed = false;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.startsWith("/api/model-checks/animations/accounts/")) {
      if (failed) {
        await route.fulfill({ status: 502, json: { detail: "绑定 Key 已失效，请更新授权后重试" } });
      } else {
        const models = [refreshed ? "refreshed-model" : "shared-model"];
        if (path.includes("/41/")) models.push("account-only-model");
        await route.fulfill({ json: { models } });
      }
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "动画模型测试" },
      "/api/accounts": ["41", "42"].map((id) => ({
        ...account,
        id,
        name: `动画账号${id}`,
        platform: "openai",
      })),
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("checkbox", { name: "检测 动画账号41" }).check();
  await page.getByRole("checkbox", { name: "检测 动画账号42" }).check();
  const button = page.getByRole("button", { name: "获取模型", exact: true });
  const input = page.getByRole("combobox", { name: "检测模型" });
  await button.click();
  await expect(button).toHaveAttribute("aria-busy", "false");
  await input.click();
  await expect(page.getByRole("option", { name: "shared-model", exact: true })).toBeVisible();
  await expect(page.getByRole("option", { name: "account-only-model" })).toHaveCount(0);
  await page.getByRole("option", { name: "shared-model", exact: true }).click();
  await expect(input).toHaveValue("shared-model");

  failed = true;
  await button.click();
  await expect(
    page.getByText("绑定 Key 已失效，请更新授权后重试", { exact: true }).first(),
  ).toBeVisible();
  await expect(input).toHaveValue("shared-model");
  failed = false;
  refreshed = true;
  await button.click();
  await expect(button).toHaveAttribute("aria-busy", "false");
  await input.fill("");
  await input.click();
  await expect(page.getByRole("option", { name: "refreshed-model" })).toBeVisible();
  await expect(page.getByRole("option", { name: "shared-model" })).toHaveCount(0);
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(input).toHaveValue("refreshed-model");
});
