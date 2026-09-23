import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("选择多个分组后才开始发现模型，未选分组不参与同步", async ({ page }, testInfo) => {
  const groups = ["基础分组", "高级分组名称较长用于验证窄屏可访问性"];
  const accounts = [
    { ...account, id: "41", name: "基础账号", groups: [groups[0]], platform: "openai" },
    { ...account, id: "42", name: "高级账号", groups: [groups[1]], platform: "openai" },
    { ...account, id: "43", name: "共享账号", groups, platform: "openai" },
    { ...account, id: "44", name: "未选账号", groups: ["不参与分组"], platform: "openai" },
  ];
  const discovery = {
    id: "discovery",
    status: "succeeded",
    operation: "account-model-discovery",
    progress: 100,
    result: {
      items: accounts
        .filter((item) => item.id !== "44")
        .map((item) => ({ account_id: item.id, status: "succeeded" })),
    },
  };
  const submissions: unknown[] = [];
  const discoveries: unknown[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    let body: unknown = pageFixtures[path] ?? {};
    if (path === "/api/setup/status") body = { initialized: true, configuration_errors: [] };
    else if (path === "/api/auth/session") body = { authenticated: true, username: "测试" };
    else if (path === "/api/accounts") body = accounts;
    else if (path.includes("dictionaries")) body = { items: [] };
    else if (path.endsWith("/models/discover")) {
      discoveries.push(route.request().postDataJSON());
      body = discovery;
    } else if (path.endsWith("/tasks/discovery")) body = discovery;
    else if (path.endsWith("/models/preview"))
      body = {
        fingerprint: "fixture",
        account_count: 3,
        accounts_with_catalog: 3,
        blocked_models: [],
        blocked_patterns: [],
        models: [{ model: "common", account_count: 3 }],
        accounts: accounts
          .filter((item) => item.id !== "44")
          .map((item) => ({
            account_id: item.id,
            account_name: item.name,
            platform: item.platform,
            models: ["common"],
            enabled_models: ["common"],
            probe_model: "",
          })),
      };
    else if (path.endsWith("/models/apply")) {
      submissions.push(route.request().postDataJSON());
      body = { ...discovery, id: "apply", result: { probe_disabled: true } };
    } else if (path.endsWith("/tasks/apply"))
      body = { ...discovery, id: "apply", result: { probe_disabled: true } };
    await route.fulfill({ json: body });
  });
  await page.goto("/accounts");
  await page.getByRole("button", { name: "账号维护" }).click();
  await page.getByRole("menuitem", { name: "同步模型", exact: true }).click();
  const scope = page.getByRole("dialog", { name: "选择同步分组" });
  await expect(scope).toBeVisible();
  await expect(page.getByRole("button", { name: "开始同步" })).toBeDisabled();
  expect(discoveries).toEqual([]);
  await page.getByRole("combobox", { name: "同步分组" }).click();
  for (const group of groups) await page.getByRole("option", { name: `${group} · 2` }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByText("已选 2 个分组，共 3 个账号")).toBeVisible();
  expect(await scope.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("select-groups-before-sync.png") });
  expect(discoveries).toEqual([]);
  await page.getByRole("button", { name: "开始同步" }).click();
  await expect(page.getByRole("checkbox", { name: "同步模型 common" })).toBeChecked();
  expect(discoveries).toEqual([{ account_ids: ["41", "42", "43"] }]);
  await expect(page.getByRole("tab", { name: /不参与分组/ })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "组合分组" })).toHaveCount(0);
  await page.getByRole("button", { name: "同步 3 个账号", exact: true }).click();
  await expect
    .poll(() => submissions)
    .toEqual([
      {
        accounts: [
          { account_id: "41", models: ["common"] },
          { account_id: "42", models: ["common"] },
          { account_id: "43", models: ["common"] },
        ],
        catalog_fingerprint: "fixture",
        probe_models: [],
      },
    ]);
});
