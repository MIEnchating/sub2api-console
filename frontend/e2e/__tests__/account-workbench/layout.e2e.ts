import { expect, test } from "@playwright/test";
import { installWorkbenchFixture, longTemplateName } from "./fixture";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

test("平板仅展示四个主入口，导航与账号操作不会超出可用宽度", async ({ page }) => {
  await page.setViewportSize({ width: 768, height: 900 });
  await installWorkbenchFixture(page);
  await page.goto("/account-workbench");
  const navigation = page.getByRole("tablist", { name: "账号工作台功能" });
  await expect(navigation.getByRole("tab")).toHaveText([
    "导入账号",
    "配置模板",
    "处理记录",
    "自动维护",
  ]);
  await page.evaluate(() => document.fonts.ready);
  for (const tab of await navigation.getByRole("tab").all()) {
    await expect(tab).toBeInViewport();
    expect(await tab.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  }
  await expect(page.getByRole("combobox", { name: "账号操作" })).toHaveCount(0);
  await page.screenshot({
    path: test.info().outputPath("tablet-navigation.png"),
    animations: "disabled",
  });
});

test("主导航支持方向键切换，账号输入合并为一个入口且窄屏可操作", async ({ page }) => {
  await installWorkbenchFixture(page);
  await page.goto("/account-workbench");
  const navigation = page.getByRole("tablist", { name: "账号工作台功能" });
  await expect(navigation.getByRole("tab")).toHaveCount(4);
  for (const tab of await navigation.getByRole("tab").all()) await expect(tab).toBeInViewport();
  expect(await navigation.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await navigation.getByRole("tab", { name: "导入账号", exact: true }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(navigation.getByRole("tab", { name: "配置模板" })).toBeFocused();
  await expect(page.getByRole("tabpanel", { name: "配置模板", exact: true })).toBeVisible();
  await page.keyboard.press("Home");
  await expect(navigation.getByRole("tab", { name: "导入账号" })).toBeFocused();
  await expect(page.getByRole("tablist", { name: "账号操作", exact: true })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "账号操作" })).toHaveCount(0);
  const output = page.getByRole("button", { name: "仅导出 JSON" });
  await output.focus();
  await page.keyboard.press("Enter");
  await expect(output).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("combobox", { name: "配置模板" })).toHaveCount(0);
  await page.screenshot({
    path: test.info().outputPath("local-navigation.png"),
    animations: "disabled",
  });
});

test("导入输入和配置在宽屏并排、手机堆叠，长模板和底部操作不溢出", async ({ page, isMobile }) => {
  await installWorkbenchFixture(page);
  await page.goto("/account-workbench");
  const input = page.getByRole("region", { name: "账号内容与文件" });
  const options = page.getByRole("region", { name: "导入选项" });
  await expect(input).toBeVisible();
  await expect(options).toBeVisible();
  const inputBox = await input.boundingBox();
  const optionsBox = await options.boundingBox();
  expect(inputBox).not.toBeNull();
  expect(optionsBox).not.toBeNull();
  if ((page.viewportSize()?.width ?? 0) >= 1200 && !isMobile) {
    expect(optionsBox!.x).toBeGreaterThan(inputBox!.x + inputBox!.width);
    expect(optionsBox!.y).toBeLessThan(inputBox!.y + inputBox!.height);
    const action = page.getByRole("button", { name: "解析并预览", exact: true });
    await expect(action).toBeInViewport();
  } else {
    expect(optionsBox!.y).toBeGreaterThan(inputBox!.y + inputBox!.height);
  }
  const template = options.getByRole("combobox", { name: "配置模板" });
  await template.click();
  await page.getByRole("option", { name: longTemplateName, exact: true }).click();
  expect(await options.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  const button = page.getByRole("button", { name: "解析并预览", exact: true });
  await button.scrollIntoViewIfNeeded();
  await expect(button).toBeInViewport();
  await expect(button).toHaveCSS("height", "32px");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await options.getByRole("button", { name: "高级设置" }).click();
  await expect(options.getByLabel("登录 / 检测代理")).toBeVisible();
  await expect(options.getByRole("combobox", { name: "短信验证码" })).toHaveCount(0);
  await expect(options.getByLabel("接码 API Key")).toHaveCount(0);
  await expect(options.getByRole("checkbox", { name: "保存本批恢复资料" })).toHaveCount(0);
  await button.scrollIntoViewIfNeeded();
  await page.screenshot({
    path: test.info().outputPath("import-layout.png"),
    animations: "disabled",
  });
});
