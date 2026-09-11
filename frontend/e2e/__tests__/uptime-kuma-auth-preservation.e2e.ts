import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";
import { config, monitor } from "../../src/features/uptime-kuma/components/__tests__/fixtures";
import { defaultMonitorOptions } from "../../src/features/uptime-kuma/lib/schemas";

for (const method of ["basic", "bearer", "ntlm"]) {
  test(`已有 ${method} 鉴权切换模板并修改其他参数后可留空保存`, async ({ page }) => {
    const fixture = await setupKuma(page);
    await page.route("**/api/uptime-kuma/monitors", (route) =>
      route.fulfill({
        json: {
          config,
          monitors: [
            {
              ...monitor,
              options: { ...defaultMonitorOptions, auth_method: method, auth_configured: true },
            },
          ],
        },
      }),
    );
    await page.goto("/uptime-kuma");
    await page.getByRole("button", { name: "编辑", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "编辑监控项", exact: true });
    for (const name of ["API JSON 模板", "手动设置", "API JSON 模板"]) {
      await dialog.getByRole("combobox", { name: "功能模板", exact: true }).click();
      await page.getByRole("option", { name, exact: true }).click();
    }
    if (method !== "ntlm") {
      await expect(dialog.getByLabel("鉴权密码 / Token（留空保留）", { exact: true })).toHaveValue(
        "",
      );
    } else {
      await dialog.getByRole("combobox", { name: "HTTP 鉴权方式" }).click();
      await expect(page.getByRole("option", { name: "保留现有鉴权" })).toBeVisible();
      await page.getByRole("option", { name: "保留现有鉴权" }).click();
    }
    await dialog.getByLabel("监控项名称").fill("改名后监控");
    await dialog.getByLabel("请求模型", { exact: true }).fill("custom-model");
    await dialog.getByRole("button", { name: "保存监控项" }).click();
    await expect(dialog).toBeHidden();
    expect(fixture.writes[0]?.value).toMatchObject({
      action: "edit",
      monitor: {
        name: "改名后监控",
        template_model: "custom-model",
        options: { auth_method: method, auth_username: "", auth_password: "", clear_auth: false },
      },
    });
  });
}
