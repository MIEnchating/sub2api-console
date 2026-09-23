import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("首排模型提示不被滚动区裁切，手动输入及右键排除保持空探活", async ({ page }, testInfo) => {
  const models = Array.from(
    { length: 40 },
    (_, index) => `model-${String(index).padStart(2, "0")}`,
  );
  let blockedPatterns = ["legacy-*"];
  const discovery = {
    id: "discovery",
    status: "succeeded",
    operation: "account-model-discovery",
    progress: 100,
    result: { items: [{ account_id: "41", account_name: "测试账号", status: "succeeded" }] },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let body: unknown = pageFixtures[path] ?? {};
    if (path === "/api/setup/status") body = { initialized: true, configuration_errors: [] };
    else if (path === "/api/auth/session") body = { authenticated: true, username: "测试" };
    else if (path === "/api/accounts")
      body = [{ ...account, id: "41", name: "测试账号", groups: ["测试组"], platform: "openai" }];
    else if (path.includes("dictionaries")) body = { items: [] };
    else if (path.endsWith("/models/discover") || path.endsWith("/tasks/discovery"))
      body = discovery;
    else if (path === "/api/config/model-sync") {
      if (route.request().method() === "PUT") {
        blockedPatterns = (route.request().postDataJSON() as { blocked_patterns: string[] })
          .blocked_patterns;
      }
      body = { blocked_patterns: blockedPatterns };
    } else if (path.endsWith("/models/preview"))
      body = {
        fingerprint: blockedPatterns.join(","),
        account_count: 1,
        accounts_with_catalog: 1,
        blocked_models: blockedPatterns.filter((pattern) => models.includes(pattern)),
        blocked_patterns: blockedPatterns,
        models: models.map((model) => ({ model, account_count: 1 })),
        accounts: [
          {
            account_id: "41",
            account_name: "测试账号",
            platform: "openai",
            models,
            enabled_models: ["model-00"],
            probe_model: "model-00",
          },
        ],
      };
    await route.fulfill({ json: body });
  });
  await page.goto("/accounts");
  await page.getByRole("button", { name: "账号维护" }).click();
  await page.getByRole("menuitem", { name: "同步模型", exact: true }).click();
  await page.getByRole("button", { name: "选择全部分组" }).click();
  await page.getByRole("button", { name: "开始同步" }).click();
  const card = page.getByRole("group", { name: "模型 model-00", exact: true });
  await expect(card).toBeVisible();
  await expect(
    page.getByRole("checkbox", { name: "同步模型 model-00", exact: true }),
  ).toBeChecked();
  await expect(
    page.getByRole("checkbox", { name: "同步模型 model-01", exact: true }),
  ).not.toBeChecked();
  expect(
    (await page.getByTestId("account-sync-models").boundingBox())!.height,
  ).toBeGreaterThanOrEqual(128);
  await card.hover();
  const tooltip = page.getByRole("tooltip").filter({ hasText: "支持账号" });
  await expect(tooltip).toBeVisible();
  const box = await tooltip.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(
    await tooltip.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      return element.contains(
        document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2),
      );
    }),
  ).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("first-row-tooltip.png") });
  await card.click({ button: "right" });
  await page.getByRole("menuitem", { name: "排除此模型（全局屏蔽）" }).click();
  await expect(page.getByRole("checkbox", { name: "同步模型 model-00", exact: true })).toHaveCount(
    0,
  );
  expect(blockedPatterns).toEqual(["legacy-*", "model-00"]);
  await expect(page.getByRole("button", { name: "添加模型" })).toBeEnabled();
  await page.getByRole("textbox", { name: "手动输入同步模型" }).fill("custom-model");
  await page.getByRole("button", { name: "添加模型" }).click();
  await expect(page.getByRole("checkbox", { name: "同步模型 custom-model" })).toBeChecked();
  await expect(page.getByRole("combobox", { name: "统一探活模型" })).toHaveText("选择探活模型");
  await expect(page.getByRole("button", { name: "同步 1 个账号", exact: true })).toBeEnabled();
  await page.getByRole("button", { name: "关闭", exact: true }).last().click();
  await page.getByRole("button", { name: "账号维护" }).click();
  await page.getByRole("menuitem", { name: "同步模型", exact: true }).click();
  await page.getByRole("button", { name: "选择全部分组" }).click();
  await page.getByRole("button", { name: "开始同步" }).click();
  await expect(
    page.getByRole("checkbox", { name: "同步模型 model-01", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "同步模型 model-00", exact: true })).toHaveCount(
    0,
  );
});
