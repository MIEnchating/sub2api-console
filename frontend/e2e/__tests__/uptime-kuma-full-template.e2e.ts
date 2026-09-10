import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";
import { templateDefaults } from "../../src/features/uptime-kuma/lib/template-schema";

test("模板保存完整检测参数并可选择 Claude CLI 请求功能", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "新增模板" }).click();
  const dialog = page.getByRole("dialog", { name: "新增功能模板" });
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await dialog.getByLabel("模板名称").fill("Claude 检测");
  await dialog.getByLabel("监控地址", { exact: true }).fill("https://monitor.example/v1/messages");
  for (const [label, value] of [
    ["检测间隔（秒）", "300"],
    ["超时（秒）", "120"],
    ["重试间隔（秒）", "180"],
    ["最大重试次数", "3"],
    ["最大重定向次数", "4"],
    ["正常状态码", "200-299, 301"],
  ])
    await dialog.getByLabel(label, { exact: true }).fill(value!);
  await dialog.getByRole("checkbox", { name: "忽略 TLS 证书错误" }).check();
  await dialog.getByRole("checkbox", { name: "反转正常 / 故障判断" }).check();
  await dialog.getByRole("combobox", { name: "接口模式" }).click();
  await page.getByRole("option", { name: "Claude CLI 请求", exact: true }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(/claude-sonnet-4-6/);
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    request_profile: "claude-cli",
    model: "claude-sonnet-4-6",
    monitoring: {
      type: "http",
      url: "https://monitor.example/v1/messages",
      interval: 300,
      timeout: 120,
      retry_interval: 180,
      max_retries: 3,
      max_redirects: 4,
      accepted_status_codes: ["200-299", "301"],
      ignore_tls: true,
      upside_down: true,
    },
  });
});

test("套用完整模板后可以调整检测参数和鉴权，仍保留所属分组", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.route("**/api/uptime-kuma/templates", async (route) =>
    route.fulfill({
      json: [
        {
          id: "c".repeat(48),
          revision: 1,
          name: "Claude CLI 请求",
          method: "POST",
          auth_method: "none",
          headers_configured: true,
          body_configured: true,
          auth_configured: false,
          request_profile: "claude-cli",
          model: "claude-sonnet-4-6",
          url_configured: true,
          monitoring: {
            ...templateDefaults().monitoring,
            url: "https://monitor.example/v1/messages",
            interval: 300,
            timeout: 120,
            retry_interval: 180,
            max_retries: 3,
            max_redirects: 4,
            ignore_tls: true,
            upside_down: true,
          },
        },
      ],
    }),
  );
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "新增监控项", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新增监控项" });
  await dialog.getByLabel("监控项名称").fill("CLI 探活");
  await dialog.getByRole("combobox", { name: "所属分组" }).click();
  await page.getByRole("option", { name: "核心分组", exact: true }).click();
  await dialog.getByRole("combobox", { name: "功能模板" }).click();
  await page.getByRole("option", { name: "Claude CLI 请求", exact: true }).click();
  await expect(dialog.getByLabel("监控地址", { exact: true })).toHaveValue(
    "https://monitor.example/v1/messages",
  );
  await expect(dialog.getByLabel("超时（秒）", { exact: true })).toHaveValue("120");
  await expect(dialog.getByRole("combobox", { name: "所属分组" })).toContainText("核心分组");
  await dialog.getByLabel("检测间隔（秒）", { exact: true }).fill("600");
  await dialog.getByRole("combobox", { name: "HTTP 鉴权方式" }).click();
  await page.getByRole("option", { name: "Bearer", exact: true }).click();
  await dialog.getByLabel("鉴权密码 / Token", { exact: true }).fill("fixture-key");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    monitor: {
      template_id: "c".repeat(48),
      template_settings_override: true,
      template_auth_override: true,
      parent: 7,
      interval: 600,
      options: { timeout: 120, auth_password: "fixture-key", max_retries: 3, ignore_tls: true },
    },
  });
});
