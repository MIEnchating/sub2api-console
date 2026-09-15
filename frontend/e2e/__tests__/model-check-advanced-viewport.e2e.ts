import { expect, test } from "@playwright/test";
import { configurationFixture, mockReadRoutes } from "./fixtures/model-check-configuration";

test("低高度窗口出现 JSON 校验错误时仍能编辑，关闭和保存始终可见", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme),
    colorScheme === "dark" ? "light" : "dark",
  );
  await mockReadRoutes(page, configurationFixture());
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  const editor = dialog.getByRole("textbox", { name: "规则与题库 JSON" });
  await expect(editor).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("advanced-alternate-theme.png") });

  await page.setViewportSize({
    width: page.viewportSize()!.width >= 768 ? 1280 : 320,
    height: 480,
  });
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.evaluate(() => navigator.clipboard.writeText("{"));
  await editor.press("Control+a");
  await editor.press("Control+v");
  await expect(editor).toHaveText("{");
  await dialog.getByRole("button", { name: "保存草稿" }).click();
  const error = dialog.getByRole("alert");
  await expect(error).toHaveText("配置必须是有效的 JSON 对象");
  await error.scrollIntoViewIfNeeded();
  await expect(error).toBeInViewport({ ratio: 1 });
  const frame = dialog.locator('[data-slot="json-editor"]');
  expect(
    await frame.locator(".cm-scroller").evaluate((element) => element.clientHeight),
  ).toBeGreaterThanOrEqual(44);
  await expect(dialog.getByRole("button", { name: "保存草稿" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "发布生效" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("advanced-low-height-validation.png") });
  await dialog.getByRole("button", { name: "恢复已保存内容" }).click();
  await expect(error).toHaveCount(0);
  await expect(editor).toHaveAttribute("aria-invalid", "false");
});
