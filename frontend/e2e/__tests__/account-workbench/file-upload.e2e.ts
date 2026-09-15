import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("文件上传按钮可用键盘打开选择器，长文件名与重选在桌面手机均可操作", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const fixture = await installWorkbenchFixture(page);
  await page.goto("/account-workbench");
  const upload = page.getByRole("group", { name: "文件上传", exact: true });
  const button = upload.getByRole("button", { name: "选择文件" });
  await expect(button).toBeVisible();
  await expect(button).toHaveCSS("height", "32px");
  await expect(upload).toHaveCSS("border-top-style", "dashed");
  await button.scrollIntoViewIfNeeded();
  await expect(button).toBeInViewport();
  await page.screenshot({
    path: test.info().outputPath("upload-empty.png"),
    animations: "disabled",
  });
  const file = {
    name: "账号资料-".repeat(20) + ".txt",
    mimeType: "text/plain",
    buffer: Buffer.from("rt_upload_fixture"),
  };
  const chooserReady = page.waitForEvent("filechooser");
  await button.focus();
  await page.keyboard.press("Enter");
  await (await chooserReady).setFiles(file);
  await expect(upload.getByRole("status")).toHaveText(file.name);
  await expect(page.getByRole("textbox", { name: "账号内容" })).toHaveValue("rt_upload_fixture");
  expect(await upload.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const reselect = upload.getByRole("button", { name: "重新选择" });
  await expect(reselect).toBeInViewport();
  await page.screenshot({
    path: test.info().outputPath("upload-selected.png"),
    animations: "disabled",
  });
  const retryReady = page.waitForEvent("filechooser");
  await reselect.click();
  await (await retryReady).setFiles(file);
  await expect(page.getByRole("textbox", { name: "账号内容" })).toHaveValue("rt_upload_fixture");
  await page.getByRole("button", { name: "清空输入", exact: true }).click();
  await expect(upload.getByRole("status")).toHaveText("未选择文件");
  expect(fixture.imports).toEqual([]);
});
