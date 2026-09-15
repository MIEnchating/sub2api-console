import { expect, test } from "@playwright/test";
import type { WorkbenchTemplate, WorkbenchTemplateSource } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("来源模板长名称在桌面和手机不溢出，预览确认前不保存且底部操作保持可见", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  let template: WorkbenchTemplate = {
    id: "template-source",
    revision: 4,
    preferred: false,
    name: "团队来源模板_" + "SharedWorkspace".repeat(6),
    priority: 0,
    match: { plan_type: "", email_domain: "" },
    config: {
      concurrency: 10,
      priority: 0,
      rate_multiplier: "1",
      group_ids: [],
      auto_pause_on_expired: true,
    },
    target_url: "https://sub2api.example.test",
    source_account_id: "42",
    source_name: "来源账号_" + "Account".repeat(14),
    source_revision: "source-old",
    source_synced_at: "2026-09-14T08:00:00Z",
  };
  const source: WorkbenchTemplateSource = {
    account_id: "42",
    account_name: template.source_name!,
    target: template.target_url!,
    source_revision: "source-new",
    synced_at: "2026-09-14T09:00:00Z",
    match: { plan_type: "plus", email_domain: "" },
    priority: 10,
    config: {
      ...template.config,
      concurrency: 25,
      rate_multiplier: "0.1234567890123456789",
      extra: { openai_ws_force_http: true },
    },
  };
  const writes: unknown[] = [];
  await page.route("**/api/accounts", (route) =>
    route.fulfill({
      json: [{ id: "42", name: template.source_name, platform: "openai", account_type: "oauth" }],
    }),
  );
  await page.route("**/api/account-workbench/templates**", async (route) => {
    if (route.request().method() === "GET") return route.fulfill({ json: [template] });
    const input = route.request().postDataJSON() as Partial<WorkbenchTemplate>;
    writes.push(input);
    template = { ...template, ...input, revision: template.revision + 1 };
    await route.fulfill({ json: template });
  });
  await page.route("**/api/account-workbench/template-from-account", async (route) => {
    expect(route.request().postDataJSON()).toEqual({ account_id: "42" });
    await route.fulfill({ json: source });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "配置模板", exact: true }).click();
  const card = page.getByRole("article", { name: `配置模板 ${template.name}` });
  await expect(card).toContainText(template.source_name!);
  expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const preference = card.getByRole("button", { name: "使用", exact: true });
  await preference.focus();
  await page.keyboard.press("Enter");
  await expect(card.getByRole("button", { name: "当前使用" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  expect(writes).toEqual([{ revision: 4, preferred: true }]);
  await card.getByRole("button", { name: "刷新来源" }).click();
  const dialog = page.getByRole("dialog", { name: "读取线上账号配置" });
  await expect(
    dialog.getByRole("definition").filter({ hasText: "0.1234567890123456789" }),
  ).toBeVisible();
  await expect(dialog.getByRole("button", { name: "保存并使用" })).toBeEnabled();
  await expect(dialog.getByRole("button", { name: "保存并使用" })).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeInViewport();
  await expect(dialog.getByRole("spinbutton")).toHaveCount(0);
  await expect(dialog.getByRole("checkbox")).toHaveCount(0);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("template-source-preview.png"),
    animations: "disabled",
  });
  expect(writes).toHaveLength(1);
  await expect(dialog.getByRole("definition").filter({ hasText: /^25$/ })).toBeVisible();
  await dialog.getByRole("button", { name: "保存并使用" }).click();
  await expect(dialog).toHaveCount(0);
  expect(writes[1]).toMatchObject({
    revision: 5,
    preferred: true,
    source_account_id: "42",
    source_revision: "source-new",
    config: { concurrency: 25 },
  });
});
