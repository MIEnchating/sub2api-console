import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("手机号步骤临时配置接码，确认费用后提交且重新打开显示原订单", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  let configured = false;
  const writes: unknown[] = [];
  await page.route("**/api/account-workbench/oauth/oauth-fixture/sms", async (route) => {
    if (route.request().method() === "POST") {
      writes.push(route.request().postDataJSON() as unknown);
      configured = true;
      await route.fulfill({ json: { accepted: true } });
      return;
    }
    await route.fulfill({
      json: {
        scope: "managed",
        stage: "phone",
        revision: "official-page-revision",
        configured,
        can_attach: !configured,
      },
    });
  });
  const run = {
    id: "sms-run",
    task_id: "sms-task",
    status: "waiting_input",
    message: "等待登录",
    current_oauth_id: "oauth-fixture",
    available: 0,
    export_only: false,
    expires_at: new Date(Date.now() + 600000).toISOString(),
    errors: [],
    items: [
      {
        index: 0,
        kind: "oauth_login",
        name: "短信账号",
        email: "sms@example.test",
        status: "running",
        message: "等待登录",
        has_password: true,
        has_totp: false,
        has_proxy: false,
      },
    ],
  };
  await page.route("**/api/account-workbench/runs/preview", (route) =>
    route.fulfill({ json: { ...run, id: "sms-preview", target: "https://sub2api.example.test" } }),
  );
  await page.route("**/api/account-workbench/runs", (route) => route.fulfill({ json: run }));
  await page.route("**/api/account-workbench/runs/sms-run", (route) =>
    route.fulfill({ json: run }),
  );
  await page.goto("/account-workbench");
  await page
    .getByRole("textbox", { name: "账号内容" })
    .fill("sms@example.test----fixture-password");
  await page.getByRole("button", { name: "解析并预览" }).click();
  await page.getByRole("button", { name: "确认处理 1 项" }).click();
  await page.getByRole("button", { name: "开始处理" }).click();
  await page.getByRole("button", { name: "配置短信接码" }).click();
  const dialog = page.getByRole("dialog", { name: "配置当前授权的短信接码" });
  await dialog.getByRole("combobox", { name: "短信验证码" }).click();
  await page.getByRole("option", { name: "LubanSMS" }).click();
  await dialog.getByLabel("接码 API Key").fill("isolated-provider-key");
  await dialog.getByLabel("LubanSMS 供应商编号").fill("isolated-service");
  await dialog.getByRole("button", { name: "确认用于当前手机号步骤" }).click();
  expect(writes).toEqual([]);
  const confirmed = dialog.getByRole("checkbox", {
    name: "我确认绑定接码手机号，并承担供应商费用",
  });
  await expect(confirmed).toHaveAttribute("aria-invalid", "true");
  await confirmed.check();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "确认用于当前手机号步骤" })).toBeInViewport();
  await page.screenshot({
    path: test.info().outputPath("live-sms-confirm.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "确认用于当前手机号步骤" }).click();
  await expect(dialog).toHaveCount(0);
  expect(writes).toEqual([
    {
      scope: "managed",
      revision: "official-page-revision",
      sms: {
        provider: "luban",
        api_key: "isolated-provider-key",
        service_id: "isolated-service",
        confirmed: true,
      },
    },
  ]);
  await page.getByRole("button", { name: "配置短信接码" }).click();
  await expect(dialog).toContainText("当前授权已有接码配置或订单");
  await expect(dialog.getByLabel("接码 API Key")).toHaveCount(0);
  await dialog.getByRole("button", { name: "返回授权页面" }).click();
});
