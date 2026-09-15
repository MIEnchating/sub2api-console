import { expect, test } from "@playwright/test";
import { configurationFixture, mockReadRoutes } from "./fixtures/model-check-configuration";

test("高级设置的查找与替换在小窗口仍保留代码空间，Escape 只关闭当前菜单或查找栏", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await mockReadRoutes(page, configurationFixture());
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  const editor = dialog.getByRole("textbox", { name: "规则与题库 JSON" });
  await editor.press("Control+f");
  const search = dialog.getByRole("search", { name: "查找与替换" });
  const input = search.getByRole("textbox", { name: "查找", exact: true });
  await input.fill("claude");
  await search.getByRole("button", { name: "展开替换" }).click();
  await search.getByRole("textbox", { name: "替换为" }).fill("model");
  await expect(dialog.locator('[data-slot="json-editor"]')).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByLabel("光标位置")).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: testInfo.outputPath("advanced-search.png") });
  await search.getByRole("button", { name: "匹配选项" }).click();
  await expect(page.getByRole("menu")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menu")).toHaveCount(0);
  await expect(search).toBeVisible();
  await input.press("Escape");
  await expect(search).toHaveCount(0);
  await expect(editor).toBeFocused();
  await expect(dialog).toBeVisible();

  await page.setViewportSize({
    width: page.viewportSize()!.width >= 768 ? 1280 : 320,
    height: 480,
  });
  await editor.press("Control+f");
  await search.getByRole("button", { name: "展开替换" }).click();
  await search.scrollIntoViewIfNeeded();
  const frame = dialog.locator('[data-slot="json-editor"]');
  expect((await frame.locator(".cm-scroller").boundingBox())!.height).toBeGreaterThanOrEqual(32);
  expect(
    await frame
      .locator(".cm-panels")
      .evaluate(
        (element) =>
          element.scrollHeight <= element.clientHeight &&
          element.scrollWidth <= element.clientWidth,
      ),
  ).toBe(true);
  await expect(search.getByRole("button", { name: "全部替换" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "保存草稿" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.screenshot({ path: testInfo.outputPath("advanced-search-low-height.png") });
});
