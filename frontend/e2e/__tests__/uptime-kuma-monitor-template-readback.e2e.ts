import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";
import { config, monitor } from "../../src/features/uptime-kuma/components/__tests__/fixtures";

for (const deleted of [false, true]) {
  test(`编辑套用模板的监控时回显关联和模型，模板${deleted ? "已删除" : "已更新"}也不重新套用`, async ({
    page,
  }) => {
    const fixture = await setupKuma(page);
    await page.route("**/api/uptime-kuma/monitors", (route) =>
      route.fulfill({
        json: {
          config,
          warning: "",
          monitors: [
            {
              ...monitor,
              name: "关联监控",
              template_id: "a".repeat(48),
              template_revision: 1,
              template_name: "旧版模板",
              template_model: "custom-model",
              template_body_encoding: "json",
              options: {
                method: "POST",
                timeout: 45,
                retry_interval: 60,
                max_retries: 0,
                resend_interval: 0,
                max_redirects: 10,
                ignore_tls: false,
                upside_down: false,
                accepted_status_codes: ["200-299"],
                notification_ids: [],
                hostname: "",
                port: 443,
                keyword: "",
                dns_record_type: "A",
                dns_resolver: "1.1.1.1",
                auth_method: "bearer",
                auth_configured: true,
                body_configured: true,
                headers_configured: true,
              },
            },
          ],
        },
      }),
    );
    if (deleted)
      await page.route("**/api/uptime-kuma/templates", (route) => route.fulfill({ json: [] }));
    await page.goto("/uptime-kuma");
    await page
      .getByRole("group", { name: "关联监控 的操作" })
      .getByRole("button", { name: "编辑" })
      .click();
    const dialog = page.getByRole("dialog", { name: "编辑监控项", exact: true });
    await expect(dialog.getByRole("combobox", { name: "功能模板" })).toContainText("旧版模板");
    await expect(dialog.getByLabel("请求模型", { exact: true })).toHaveValue("custom-model");
    await expect(dialog.getByLabel("鉴权密码 / Token（留空保留）", { exact: true })).toHaveValue(
      "",
    );
    await dialog.getByLabel("监控项名称").fill("只改名称");
    await dialog.getByRole("button", { name: "保存监控项" }).click();
    await expect(dialog).toBeHidden();
    expect(fixture.writes[0]?.value).toMatchObject({
      monitor: {
        template_id: "a".repeat(48),
        template_revision: 1,
        template_model: "custom-model",
        template_retain: true,
        options: { auth_password: "", timeout: 45 },
      },
    });
  });
}
