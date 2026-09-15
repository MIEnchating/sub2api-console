import { expect, test } from "@playwright/test";
import { installWorkbenchFixture, longTemplateName } from "./fixture";

test("授权页在桌面与手机可操作浏览器、选择长模板并确认导入", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const fixture = await installWorkbenchFixture(page);
  await page.goto("/account-workbench");
  const importTab = page.getByRole("tab", { name: "导入账号", exact: true });
  await importTab.focus();
  await page.keyboard.press("ArrowRight");
  const mixedTab = page.getByRole("tab", { name: "混合运行", exact: true });
  await expect(mixedTab).toHaveAttribute("aria-selected", "true");
  await expect(mixedTab).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await expect(page.getByRole("tab", { name: "授权登录", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await page.getByRole("button", { name: "开始授权登录" }).click();
  const surface = page.getByRole("button", { name: "上游登录页面" });
  await expect(surface).toBeVisible();
  await expect(surface.getByRole("img")).toHaveJSProperty("naturalWidth", 1100);
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await expect(page.getByRole("button", { name: "预览授权账号" })).toHaveCount(0);
  await page.screenshot({
    path: test.info().outputPath("oauth-browser.png"),
    animations: "disabled",
  });
  await surface.focus();
  await page.keyboard.type("hello");
  await expect
    .poll(() =>
      fixture.inputs
        .filter((input) => input.kind === "text")
        .map((input) => input.text)
        .join(""),
    )
    .toBe("hello");
  await page.keyboard.press("Tab");
  const previousField = page.getByRole("button", { name: "上一个输入框" });
  await expect(previousField).toBeFocused();
  await previousField.scrollIntoViewIfNeeded();
  await expect(previousField).toBeInViewport();
  const finish = page.getByRole("button", { name: "登录完成，验证授权" });
  await finish.scrollIntoViewIfNeeded();
  await expect(finish).toBeInViewport();
  await finish.click();
  await expect(page.getByRole("button", { name: "预览授权账号" })).toBeVisible();
  await page.getByRole("combobox", { name: "配置模板" }).click();
  const option = page.getByRole("option", { name: longTemplateName, exact: true });
  await expect(option).toBeVisible();
  expect(await option.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await option.click();
  await page.getByRole("button", { name: "预览授权账号" }).click();
  await expect(page.getByRole("table", { name: "账号预览" })).toBeVisible();
  const templateCell = page.getByRole("cell", { name: longTemplateName, exact: true });
  await expect(templateCell).toHaveCSS("white-space", "normal");
  expect(await templateCell.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await expect(page.getByRole("cell", { name: "更新凭据（ID 42）" })).toBeVisible();
  await page.getByRole("button", { name: "确认导入 1 个账号" }).click();
  const confirm = page.getByRole("dialog", { name: "确认批量导入账号" });
  await expect(confirm).toContainText("更新已有账号 1 个（ID：42）");
  await expect(confirm).toContainText(longTemplateName);
  expect(await confirm.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(fixture.imports).toHaveLength(0);
  await page.screenshot({
    path: test.info().outputPath("oauth-confirm.png"),
    animations: "disabled",
  });
  await confirm.getByRole("button", { name: "创建导入任务" }).click();
  await expect
    .poll(() => fixture.imports)
    .toEqual([{ preview_id: "oauth-preview", confirmed: true }]);
  await expect(page.getByRole("status", { name: "等待导入授权账号" })).toBeVisible();
  await expect(page.getByRole("progressbar")).toHaveCount(0);
});

test("切换工作台标签会结束授权会话并移除浏览器画面", async ({ page }) => {
  const fixture = await installWorkbenchFixture(page);
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("button", { name: "开始授权登录" }).click();
  await expect(page.getByRole("button", { name: "上游登录页面" })).toBeVisible();
  await page.getByRole("tab", { name: "导入账号", exact: true }).click();
  await expect.poll(() => fixture.cancelled).toEqual(["oauth-fixture"]);
  await expect(page.getByRole("button", { name: "上游登录页面" })).toHaveCount(0);
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await expect(page.getByRole("button", { name: "开始授权登录" })).toBeVisible();
  await expect(page.getByRole("button", { name: "上游登录页面" })).toHaveCount(0);
});
