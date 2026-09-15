import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("短信授权在桌面与手机选择真实报价并确认绑定费用后启动", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const countryTitle = "接码国家_" + "VeryLongCountryName".repeat(8);
  const price = "0.000000000000000000001";
  let startBody: unknown;
  let optionsBody: unknown;
  await page.route("**/api/account-workbench/oauth", async (route) => {
    startBody = route.request().postDataJSON() as unknown;
    await route.fallback();
  });
  await page.route("**/api/account-workbench/sms/options", async (route) => {
    optionsBody = route.request().postDataJSON() as unknown;
    await route.fulfill({
      json: [{ country: "15", title: countryTitle, iso: "XX", prefix: "1202", price, count: 3 }],
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("checkbox", { name: "自动填写登录信息" }).check();
  await page.getByRole("button", { name: "开始授权登录" }).click();
  const dialog = page.getByRole("dialog", { name: "自动填写登录信息" });
  await dialog.getByRole("textbox", { name: "登录邮箱" }).fill("operator@example.test");
  await dialog.getByRole("combobox", { name: "短信验证码" }).click();
  await page.getByRole("option", { name: "SMSBower", exact: true }).click();
  await expect(dialog.getByRole("combobox", { name: "接码国家" })).toBeDisabled();
  await dialog.getByLabel("接码 API Key").fill("fixture-key");
  await dialog.getByRole("button", { name: "读取国家价格" }).click();
  await expect(dialog.getByRole("combobox", { name: "接码国家" })).toBeEnabled();
  expect(optionsBody).toEqual({ provider: "smsbower", api_key: "fixture-key" });
  await dialog.getByRole("combobox", { name: "接码国家" }).click();
  const country = page.getByRole("option", { name: new RegExp(countryTitle) });
  await expect(country).toBeVisible();
  expect(await country.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await country.click();
  await dialog.getByRole("button", { name: "开始授权登录" }).click();
  const confirmed = dialog.getByRole("checkbox", {
    name: "我确认绑定接码手机号，并承担供应商费用",
  });
  await expect(confirmed).toHaveAttribute("aria-invalid", "true");
  expect(startBody).toBeUndefined();
  await confirmed.check();
  await expect(dialog.getByRole("button", { name: "开始授权登录" })).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("oauth-sms-confirm.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "开始授权登录" }).click();
  await expect
    .poll(() => startBody)
    .toEqual({
      login: {
        email: "operator@example.test",
        sms: {
          provider: "smsbower",
          api_key: "fixture-key",
          country: "15",
          max_price: price,
          confirmed: true,
        },
      },
    });
  await expect(page.getByRole("button", { name: "上游登录页面" })).toBeVisible();
  await expect(dialog).toHaveCount(0);
});
