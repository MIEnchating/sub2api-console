import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

for (const mode of [
  {
    label: "Claude Messages",
    profile: "claude-messages",
    endpoint: "/v1/messages",
    model: "claude-sonnet-4-6",
  },
  {
    label: "OpenAI Chat Completions",
    profile: "openai-chat",
    endpoint: "/v1/chat/completions",
    model: "gpt-4.1-mini",
  },
  {
    label: "OpenAI Responses",
    profile: "openai-responses",
    endpoint: "/v1/responses",
    model: "gpt-4.1-mini",
  },
]) {
  test(`选择 ${mode.label} 时显示接口提示和可编辑请求体，移除模板鉴权`, async ({ page }) => {
    const fixture = await setupKuma(page);
    await page.goto("/uptime-kuma/templates");
    await page.getByRole("button", { name: "新增模板" }).click();
    const dialog = page.getByRole("dialog", { name: "新增功能模板" });
    await dialog.getByRole("button", { name: "查看请求体" }).click();
    await dialog.getByLabel("模板名称").fill(mode.label);
    await dialog.getByRole("combobox", { name: "接口模式" }).click();
    await page.getByRole("option", { name: mode.label, exact: true }).click();
    await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(new RegExp(mode.model));
    await dialog
      .getByLabel("监控地址", { exact: true })
      .fill(`https://monitor.example${mode.endpoint}`);
    await dialog.getByLabel("请求头（JSON）").fill('{"X-Custom":"fixture"}');
    await expect(dialog.getByRole("combobox", { name: "鉴权方式", exact: true })).toHaveCount(0);
    await dialog.getByRole("button", { name: "保存模板" }).click();
    await expect(dialog).toBeHidden();
    expect(fixture.writes[0]?.value).toMatchObject({
      request_profile: mode.profile,
      model: mode.model,
      method: "POST",
      auth_method: "none",
      auth_password: "",
      headers: '{"X-Custom":"fixture"}',
    });
  });
}

test("切换请求模式填入对应请求体，切回自定义保留可编辑内容", async ({ page }) => {
  await setupKuma(page);
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "新增模板" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await dialog.getByRole("combobox", { name: "接口模式" }).click();
  await page.getByRole("option", { name: "OpenAI Chat Completions", exact: true }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(/max_completion_tokens/);
  await dialog.getByRole("combobox", { name: "接口模式" }).click();
  await page.getByRole("option", { name: "OpenAI Responses", exact: true }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(/max_output_tokens/);
  await dialog.getByRole("combobox", { name: "接口模式" }).click();
  await page.getByRole("option", { name: "自定义请求", exact: true }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toBeEditable();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(/max_output_tokens/);
});
