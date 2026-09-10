import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

test("接口模式补全地址，模型和消息同步到默认收起的请求体", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "新增模板" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("模板名称").fill("协议检测");
  await expect(dialog.getByRole("combobox", { name: "地址协议" })).toHaveCount(0);
  await dialog.getByLabel("监控地址", { exact: true }).fill("https://monitor.example/v1");
  await dialog.getByRole("combobox", { name: "接口模式" }).click();
  await page.getByRole("option", { name: "OpenAI Responses", exact: true }).click();
  await expect(dialog.getByLabel("监控地址", { exact: true })).toHaveValue(
    "https://monitor.example/v1/responses",
  );
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveCount(0);
  await expect(dialog.getByText(/预设已填入|保存以当前内容为准|Claude 服务需要/)).toHaveCount(0);
  await dialog.getByLabel("请求模型", { exact: true }).fill("custom-model");
  await dialog.getByLabel("发送消息", { exact: true }).fill("检查服务是否正常");
  const toggle = dialog.getByRole("button", { name: "查看请求体" });
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await toggle.click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(/custom-model/);
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(/检查服务是否正常/);
  await dialog.getByRole("button", { name: "收起请求体" }).click();
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    model: "custom-model",
    monitoring: { url: "https://monitor.example/v1/responses" },
  });
  expect(JSON.parse(fixture.writes[0]!.value.body as string)).toMatchObject({
    model: "custom-model",
    input: "检查服务是否正常",
    max_output_tokens: 16,
  });
});
