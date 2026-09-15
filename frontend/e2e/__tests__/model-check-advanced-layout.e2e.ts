import { expect, test } from "@playwright/test";
import { configurationFixture, mockReadRoutes } from "./fixtures/model-check-configuration";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

test("高级设置将高度留给编辑区，版本说明和底部操作无需滚动即可使用", async ({ page }, testInfo) => {
  await mockReadRoutes(page, configurationFixture());
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  const panel = dialog.getByRole("tabpanel", { name: "高级设置" });
  const editor = panel.locator('[data-slot="json-editor"]');
  await expect(panel.getByRole("textbox", { name: "规则与题库 JSON" })).toBeVisible();

  await expect(editor).toBeInViewport({ ratio: 1 });
  expect(await panel.evaluate((element) => element.scrollHeight <= element.clientHeight)).toBe(
    true,
  );
  const editorBounds = (await editor.boundingBox())!;
  const panelBounds = (await panel.boundingBox())!;
  expect(editorBounds.height).toBeGreaterThan(panelBounds.height / 2);
  await expect(panel.getByRole("textbox", { name: "版本说明" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "保存草稿" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "发布生效" })).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: testInfo.outputPath("advanced-settings.png") });

  await panel.getByRole("textbox", { name: "规则与题库 JSON" }).press("Control+End");
  expect(await editor.boundingBox()).toEqual(editorBounds);
  expect(await panel.evaluate((element) => element.scrollTop)).toBe(0);
});

test("打开空版本历史再返回编辑器时保留输入和键盘焦点", async ({ page }) => {
  const configuration = configurationFixture();
  await mockReadRoutes(page, configuration);
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  await dialog.getByRole("textbox", { name: "版本说明" }).fill("待保存的说明");
  const editor = dialog.getByRole("textbox", { name: "规则与题库 JSON" });
  await editor.fill('{"暂存内容":true}');
  const historyButton = dialog.getByRole("button", { name: "版本历史", exact: true });
  await historyButton.click();
  const history = page.getByRole("dialog", { name: "版本历史", exact: true });
  await expect(history.getByText("暂无历史版本")).toBeInViewport({ ratio: 1 });
  await expect(history.getByText(configuration.active.fingerprint, { exact: true })).toBeVisible();
  await history.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(historyButton).toBeFocused();
  await expect(dialog.getByRole("textbox", { name: "版本说明" })).toHaveValue("待保存的说明");
  await expect(editor).toHaveText('{"暂存内容":true}');
});

test("草稿和长版本历史不挤压编辑区，历史可滚动到末项并确认恢复", async ({ page }, testInfo) => {
  const configuration = configurationFixture();
  configuration.draft = {
    ...configuration.active,
    id: "draft-" + "long-version-".repeat(12),
    status: "draft",
    fingerprint: "draft-fingerprint",
  };
  configuration.history = Array.from({ length: 12 }, (_, index) => ({
    ...configuration.active,
    id: `history-${index}`,
    note: `历史说明 ${index}：${"long-note-".repeat(24)}`,
    fingerprint: `fingerprint-${index}`,
    claude_profiles: 1,
    probe_count: 8,
  }));
  await mockReadRoutes(page, configuration);
  await page.route("**/api/model-checks/configuration/restore", async (route) => {
    expect(route.request().postDataJSON()).toMatchObject({
      version_id: "history-11",
      expected_fingerprint: "draft-fingerprint",
    });
    await route.fulfill({
      json: {
        ...configuration,
        draft: { ...configuration.draft, note: "恢复自版本 history-11" },
      },
    });
  });
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  await expect(dialog.locator('[data-slot="json-editor"]')).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "删除草稿" })).toBeInViewport({ ratio: 1 });
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await dialog.getByRole("button", { name: "版本历史", exact: true }).click();
  const history = page.getByRole("dialog", { name: "版本历史", exact: true });
  const lastVersion = history.getByRole("listitem").last();
  await lastVersion.getByRole("button", { name: "恢复为草稿" }).scrollIntoViewIfNeeded();
  await expect(lastVersion.getByText(configuration.history[11].note)).toBeVisible();
  await expect(lastVersion.getByRole("button", { name: "恢复为草稿" })).toBeInViewport({
    ratio: 1,
  });
  expect(await history.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: testInfo.outputPath("version-history.png") });
  await lastVersion.getByRole("button", { name: "恢复为草稿" }).click();
  await page
    .getByRole("dialog", { name: "恢复历史规则", exact: true })
    .getByRole("button", { name: "恢复为草稿" })
    .click();
  await expect(dialog.getByRole("textbox", { name: "版本说明" })).toHaveValue(
    "恢复自版本 history-11",
  );
  await expect(dialog.getByRole("button", { name: "发布生效" })).toBeEnabled();
});
