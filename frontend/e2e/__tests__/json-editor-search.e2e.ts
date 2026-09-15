import { expect, test } from "@playwright/test";
import { useJsonEditorFixture } from "./fixtures/json-editor-harness";

useJsonEditorFixture();

test("查找默认收起替换，窄容器中展开后所有工具可见且面板无需滚动", async ({ page }, testInfo) => {
  await page.locator("#editor-root").evaluate((element) => {
    element.style.maxWidth = "272px";
  });
  const frame = page.locator('[data-slot="json-editor"]').first();
  await frame.getByRole("button", { name: "查找", exact: true }).click();
  const panel = frame.getByRole("search", { name: "查找与替换" });
  await expect(panel.getByRole("textbox", { name: "查找", exact: true })).toBeFocused();
  await expect(panel.getByRole("textbox", { name: "替换为" })).toHaveCount(0);
  await panel.getByRole("button", { name: "展开替换" }).click();
  await expect(panel.getByRole("button", { name: "收起替换" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  await expect(panel.getByRole("textbox", { name: "替换为" })).toBeVisible();
  for (const name of [
    "上一处",
    "下一处",
    "选中全部匹配",
    "匹配选项",
    "替换当前匹配",
    "全部替换",
    "关闭查找",
  ]) {
    await expect(panel.getByRole("button", { name, exact: true })).toBeInViewport({ ratio: 1 });
  }
  expect(
    await panel.evaluate(
      (element) =>
        element.scrollWidth <= element.clientWidth && element.scrollHeight <= element.clientHeight,
    ),
  ).toBe(true);
  expect(
    await frame
      .locator(".cm-panels")
      .evaluate((element) => element.scrollHeight <= element.clientHeight),
  ).toBe(true);
  expect((await frame.locator(".cm-scroller").boundingBox())!.height).toBeGreaterThanOrEqual(32);
  await page.screenshot({ path: testInfo.outputPath("search-narrow-expanded.png") });
});

test("查找与替换使用真实匹配结果，支持上下一处、全部选中和撤销", async ({ page }, testInfo) => {
  const frame = page.locator('[data-slot="json-editor"]').first();
  const editor = frame.getByRole("textbox", { name: "请求配置" });
  await editor.fill('{"items":["alpha","alpha","alpha"]}');
  await editor.press("Control+Home");
  await editor.press("Control+f");
  const panel = frame.getByRole("search", { name: "查找与替换" });
  const query = panel.getByRole("textbox", { name: "查找", exact: true });
  await query.fill("alpha");
  await expect(panel.getByRole("status")).toHaveText("3 处");
  await query.press("Enter");
  await expect(frame.getByLabel("光标位置")).toHaveText("1:17");
  await query.press("Enter");
  await expect(frame.getByLabel("光标位置")).toHaveText("1:25");
  await query.press("Shift+Enter");
  await expect(frame.getByLabel("光标位置")).toHaveText("1:17");
  await panel.getByRole("button", { name: "选中全部匹配" }).click();
  await expect(frame.locator(".cm-selectionBackground")).toHaveCount(3);
  await query.focus();
  await query.press("Enter");
  await panel.getByRole("button", { name: "展开替换" }).click();
  await panel.getByRole("textbox", { name: "替换为" }).fill("beta");
  await panel.getByRole("button", { name: "替换当前匹配" }).click();
  await expect(panel.getByRole("status")).toHaveText("2 处");
  await panel.getByRole("button", { name: "全部替换" }).click();
  await expect(editor).toHaveText('{"items":["beta","beta","beta"]}');
  await expect(panel.getByRole("status")).toHaveText("无匹配");
  await expect(page.getByLabel("保存结果")).toBeEmpty();
  await frame.getByRole("button", { name: "撤销", exact: true }).click();
  await expect(panel.getByRole("status")).toHaveText("2 处");
  await page.screenshot({ path: testInfo.outputPath("search-replace.png") });
});

test("匹配菜单支持键盘切换大小写和全字，正则无效时禁止查找和替换", async ({ page }, testInfo) => {
  const frame = page.locator('[data-slot="json-editor"]').first();
  await frame
    .getByRole("textbox", { name: "请求配置" })
    .fill('{"items":["Alpha","alpha","alphabet"]}');
  await frame.getByRole("button", { name: "查找", exact: true }).click();
  const panel = frame.getByRole("search", { name: "查找与替换" });
  const query = panel.getByRole("textbox", { name: "查找", exact: true });
  await query.fill("alpha");
  await expect(panel.getByRole("status")).toHaveText("3 处");
  await panel.getByRole("button", { name: "匹配选项" }).click();
  const caseOption = page.getByRole("menuitemcheckbox", { name: "区分大小写" });
  await caseOption.focus();
  await caseOption.press("Space");
  await expect(caseOption).toHaveAttribute("aria-checked", "true");
  await expect(panel.getByRole("status")).toHaveText("2 处");
  await page.getByRole("menuitemcheckbox", { name: "全字匹配" }).click();
  await expect(panel.getByRole("status")).toHaveText("1 处");
  await page.getByRole("menuitemcheckbox", { name: "正则表达式" }).click();
  await page.screenshot({ path: testInfo.outputPath("search-options.png") });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menu")).toHaveCount(0);
  await expect(panel).toBeVisible();
  await query.fill("[");
  await expect(query).toHaveAttribute("aria-invalid", "true");
  await expect(panel.getByRole("button", { name: "下一处", exact: true })).toBeDisabled();
  await panel.getByRole("button", { name: "展开替换" }).click();
  await expect(panel.getByRole("button", { name: "全部替换" })).toBeDisabled();
  await query.fill("alpha");
  await expect(query).toHaveAttribute("aria-invalid", "false");
  await expect(panel.getByRole("status")).toHaveText("1 处");
  await query.press("Enter");
  await query.fill("[");
  const replacement = panel.getByRole("textbox", { name: "替换为" });
  await replacement.focus();
  await replacement.press("F3");
  await expect(query).toHaveValue("[");
  await replacement.press("Control+g");
  await expect(query).toHaveValue("[");
});

test("关闭查找恢复编辑焦点，重新打开保留查询与替换输入", async ({ page }) => {
  const frame = page.locator('[data-slot="json-editor"]').first();
  const editor = frame.getByRole("textbox", { name: "请求配置" });
  await editor.press("Control+f");
  const panel = frame.getByRole("search", { name: "查找与替换" });
  await panel.getByRole("textbox", { name: "查找", exact: true }).fill("missing");
  await panel.getByRole("button", { name: "展开替换" }).click();
  const replacement = panel.getByRole("textbox", { name: "替换为" });
  await replacement.fill("replacement");
  await replacement.press("Escape");
  await expect(panel).toHaveCount(0);
  await expect(editor).toBeFocused();
  await editor.press("Control+f");
  await expect(panel.getByRole("textbox", { name: "查找", exact: true })).toHaveValue("missing");
  await panel.getByRole("button", { name: "展开替换" }).click();
  await expect(replacement).toHaveValue("replacement");
  await panel.getByRole("button", { name: "关闭查找" }).click();
  await expect(editor).toBeFocused();
});

test("匹配超过统计上限时显示 1000+，全部替换仍覆盖全部内容", async ({ page }) => {
  const frame = page.locator('[data-slot="json-editor"]').first();
  const editor = frame.getByRole("textbox", { name: "请求配置" });
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.evaluate(() =>
    navigator.clipboard.writeText(JSON.stringify(Array(1002).fill("alpha"))),
  );
  await editor.press("Control+a");
  await editor.press("Control+v");
  await frame.getByRole("button", { name: "查找", exact: true }).click();
  const panel = frame.getByRole("search", { name: "查找与替换" });
  await panel.getByRole("textbox", { name: "查找", exact: true }).fill("alpha");
  await expect(panel.getByRole("status")).toHaveText("1000+ 处");
  await panel.getByRole("button", { name: "展开替换" }).click();
  await panel.getByRole("textbox", { name: "替换为" }).fill("beta");
  await panel.getByRole("button", { name: "全部替换" }).click();
  await expect(panel.getByRole("status")).toHaveText("无匹配");
  await frame.getByRole("button", { name: "复制内容", exact: true }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    JSON.stringify(Array(1002).fill("beta")),
  );
});

test("只读内容允许查找且没有替换入口，禁用编辑器会关闭已打开的查找", async ({ page }) => {
  const readonlyFrame = page.locator('[data-slot="json-editor"]').last();
  await readonlyFrame.getByRole("button", { name: "查找", exact: true }).click();
  const readonlyPanel = readonlyFrame.getByRole("search", { name: "查找与替换" });
  await readonlyPanel.getByRole("textbox", { name: "查找", exact: true }).fill("original");
  await expect(readonlyPanel.getByRole("status")).toHaveText("1 处");
  await expect(readonlyPanel.getByRole("button", { name: "展开替换" })).toHaveCount(0);
  const frame = page.locator('[data-slot="json-editor"]').first();
  await frame.getByRole("button", { name: "查找", exact: true }).click();
  await page.getByRole("button", { name: "禁用", exact: true }).click();
  await expect(frame.getByRole("search")).toHaveCount(0);
  await expect(frame.getByRole("button", { name: "查找", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "启用", exact: true }).click();
  await frame.getByRole("button", { name: "查找", exact: true }).click();
  await expect(frame.getByRole("search")).toBeVisible();
});
