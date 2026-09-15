import { expect, test } from "@playwright/test";
import { useJsonEditorFixture } from "./fixtures/json-editor-harness";

useJsonEditorFixture();

test("格式化保留数值原文，撤销重做和重置正确同步表单", async ({ page }) => {
  const editor = page.getByRole("textbox", { name: "请求配置" });
  await page.getByRole("button", { name: "格式化 JSON" }).click();
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByLabel("保存结果")).toHaveText(
    '{\n  "id": 9007199254740993,\n  "price": 1.23000e-10\n}',
  );
  await page.getByRole("button", { name: "撤销" }).click();
  await expect(editor).toHaveText('{"id":9007199254740993,"price":1.23000e-10}');
  await page.getByRole("button", { name: "重做" }).click();
  await expect(editor).toContainText('"price": 1.23000e-10');
  await page.getByRole("button", { name: "重置" }).click();
  await expect(editor).toHaveText('{"reset":true}');
  await expect(page.getByRole("status").filter({ hasText: "已保存" })).toBeVisible();
});

test("语法错误可定位且保留输入，修正后解除错误状态并允许 Tab 离开", async ({ page }) => {
  const editor = page.getByRole("textbox", { name: "请求配置" });
  await editor.fill('{"a":}');
  await expect(editor).toHaveAttribute("aria-invalid", "true");
  await page.getByRole("button", { name: "格式化 JSON" }).click();
  await expect(editor).toHaveText('{"a":}');
  await expect(page.getByText("JSON 格式有误，请修正后再格式化")).toBeVisible();
  await editor.fill('{"a":1}');
  await expect(editor).toHaveAttribute("aria-invalid", "false");
  await editor.press("Tab");
  await expect(page.getByRole("button", { name: "保存", exact: true })).toBeFocused();
});

test("窄容器中工具操作完整可见，行列状态与工具栏分开且编辑时不移动", async ({ page }, testInfo) => {
  await page.locator("#editor-root").evaluate((element) => {
    element.style.maxWidth = "272px";
  });
  const frame = page.locator('[data-slot="json-editor"]').first();
  const toolbar = frame.getByRole("group", { name: "编辑器工具" });
  const position = frame.getByLabel("光标位置");
  await expect(position).toBeVisible();
  const toolbarBounds = (await toolbar.boundingBox())!;
  const positionBounds = (await position.boundingBox())!;
  expect(positionBounds.y).toBeGreaterThan(toolbarBounds.y + toolbarBounds.height);
  for (const name of ["撤销", "重做", "格式化 JSON", "查找", "复制内容"]) {
    await expect(toolbar.getByRole("button", { name, exact: true })).toBeInViewport({ ratio: 1 });
  }
  expect(await frame.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await toolbar.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await frame
    .getByRole("textbox", { name: "请求配置" })
    .fill('{"long":"' + "value-".repeat(100) + '"}');
  await frame.getByRole("textbox", { name: "请求配置" }).press("Control+End");
  await expect(position).toHaveText("1:612");
  expect(await toolbar.boundingBox()).toEqual(toolbarBounds);
  expect(await toolbar.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: testInfo.outputPath("json-editor-narrow.png") });
});

test("禁用和只读状态阻止改写，复制保留原始空白并提示剪贴板失败", async ({ page }) => {
  await page.getByRole("button", { name: "禁用", exact: true }).click();
  const editor = page.getByRole("textbox", { name: "请求配置" });
  await expect(editor).toHaveAttribute("aria-disabled", "true");
  await expect(page.getByRole("button", { name: "格式化 JSON" })).toBeDisabled();
  const readonly = page.getByRole("textbox", { name: "只读配置" });
  await expect(readonly).toHaveAttribute("aria-readonly", "true");
  await readonly.focus();
  await page.keyboard.type("modified");
  await expect(readonly).toHaveText('{"original":1e-10}');
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.getByRole("button", { name: "复制内容" }).last().click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('{"original":1e-10}\n');
  await page.evaluate(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: () => Promise.reject(new Error("denied")) },
    });
  });
  await page.getByRole("button", { name: "复制内容" }).last().click();
  await expect(page.getByText("复制失败，请允许剪贴板权限后重试")).toBeVisible();
});

test("长单行与多行内容只在编辑区滚动，查找和折叠在窄屏可用", async ({ page }, testInfo) => {
  const editor = page.getByRole("textbox", { name: "请求配置" });
  const source = JSON.stringify(
    {
      long: "value-".repeat(300),
      items: Array.from({ length: 2000 }, (_, index) => ({ id: index })),
    },
    null,
    2,
  );
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.evaluate((text) => navigator.clipboard.writeText(text), source);
  await editor.press("Control+a");
  await editor.press("Control+v");
  const frame = page.locator('[data-slot="json-editor"]').first();
  await expect(frame).toBeInViewport({ ratio: 1 });
  expect(await frame.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await editor.press("Control+End");
  await expect
    .poll(() => frame.locator(".cm-scroller").evaluate((element) => element.scrollTop))
    .toBeGreaterThan(0);
  await expect(frame.getByRole("button", { name: "格式化 JSON" })).toBeInViewport({ ratio: 1 });
  await frame.getByRole("button", { name: "查找" }).click();
  await expect(frame.getByPlaceholder("查找")).toBeVisible();
  expect(await frame.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await frame.getByPlaceholder("查找").fill("1999");
  await frame.getByPlaceholder("查找").press("Enter");
  await expect(editor).toContainText("1999");
  await frame.getByPlaceholder("查找").press("Escape");
  await editor.press("Control+Home");
  await frame.locator('.cm-foldGutter [title="折叠此行"]').first().click();
  await expect(frame.locator(".cm-foldPlaceholder")).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("json-editor.png") });
});
