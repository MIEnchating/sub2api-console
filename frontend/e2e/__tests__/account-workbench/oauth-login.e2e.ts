import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("自动授权邮箱配置在桌面和手机可输入 JSON 且开始后清除凭据表单", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  let startBody: unknown;
  await page.route("**/api/account-workbench/oauth", async (route) => {
    startBody = route.request().postDataJSON() as unknown;
    await route.fallback();
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("checkbox", { name: "自动填写登录信息" }).check();
  await page.getByRole("button", { name: "开始授权登录" }).click();
  const dialog = page.getByRole("dialog", { name: "自动填写登录信息" });
  await dialog.getByRole("textbox", { name: "登录邮箱" }).fill("operator@example.test");
  await dialog.getByLabel("登录密码").fill("fixture-password");
  await dialog
    .getByLabel("登录代理", { exact: true })
    .fill("socks5://user:fixture-proxy@proxy.example.test:1080");
  await dialog.getByRole("combobox", { name: "邮箱验证码" }).click();
  await page.getByRole("option", { name: "HTTP 邮箱", exact: true }).click();
  await dialog
    .getByRole("textbox", { name: "邮箱接口地址" })
    .fill("https://mail.example.test/inbox");
  await dialog.getByRole("combobox", { name: "请求方法" }).click();
  await page.getByRole("option", { name: "POST", exact: true }).click();
  await dialog
    .getByRole("textbox", { name: "邮箱请求头 JSON" })
    .fill('{"X-Mailbox-Key":"fixture-key"}');
  const body = '{ "mailbox": 9007199254740993 }';
  await dialog.getByRole("textbox", { name: "邮箱请求体 JSON" }).fill(body);
  await expect(dialog.getByRole("button", { name: "开始授权登录" })).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeInViewport();
  await expect(dialog.getByRole("heading", { name: "自动填写登录信息" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("oauth-mailbox-form.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "开始授权登录" }).click();
  await expect
    .poll(() => startBody)
    .toEqual({
      login: {
        email: "operator@example.test",
        password: "fixture-password",
        proxy_url: "socks5://user:fixture-proxy@proxy.example.test:1080",
        mailbox: {
          kind: "http",
          method: "POST",
          url: "https://mail.example.test/inbox",
          headers: { "X-Mailbox-Key": "fixture-key" },
          body,
        },
      },
    });
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("button", { name: "上游登录页面" })).toBeVisible();
  await page.getByRole("button", { name: "结束授权" }).click();
  await page.getByRole("checkbox", { name: "自动填写登录信息" }).check();
  await page.getByRole("button", { name: "开始授权登录" }).click();
  await expect(dialog.getByLabel("登录密码")).toHaveValue("");
  await expect(dialog.getByLabel("登录代理", { exact: true })).toHaveValue("");
  await expect(dialog.getByRole("textbox", { name: "登录邮箱" })).toHaveValue("");
  await expect(dialog.getByRole("combobox", { name: "邮箱验证码" })).toContainText("人工填写");
});
