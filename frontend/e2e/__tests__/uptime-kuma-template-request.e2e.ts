import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

test("新模板请求头为空，可填写完整地址并保存表单编码和请求体", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "新增模板" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await dialog.getByLabel("模板名称").fill("表单检测");
  await expect(dialog.getByLabel("请求头（JSON）")).toHaveValue("");
  await expect(dialog.getByRole("combobox", { name: "鉴权方式", exact: true })).toHaveCount(0);
  await dialog.getByLabel("监控地址", { exact: true }).fill("http://monitor.example/probe");
  await dialog.getByRole("combobox", { name: "请求体编码" }).click();
  await page.getByRole("option", { name: "表单（x-www-form-urlencoded）", exact: true }).click();
  await dialog.getByLabel("请求体", { exact: true }).fill("input=ping&model=custom");
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    monitoring: { url: "http://monitor.example/probe" },
    headers: "",
    body_encoding: "form",
    body: "input=ping&model=custom",
    auth_method: "none",
    clear_auth: true,
  });
});

test("内置请求体允许编辑，JSON 无效时阻止保存，切换 XML 后按原文提交", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "新增模板" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await dialog.getByLabel("模板名称").fill("自定义预设");
  await dialog.getByRole("combobox", { name: "接口模式" }).click();
  await page.getByRole("option", { name: "OpenAI Responses", exact: true }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(/max_output_tokens/);
  await expect(dialog.getByLabel("请求头（JSON）")).toHaveValue("");
  await dialog.getByLabel("请求体", { exact: true }).fill("<probe>ping</probe>");
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(fixture.writes).toHaveLength(0);
  await dialog.getByRole("combobox", { name: "请求体编码" }).click();
  await page.getByRole("option", { name: "XML", exact: true }).click();
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    request_profile: "openai-responses",
    body: "<probe>ping</probe>",
    body_encoding: "xml",
  });
});

test("预设加载中禁止保存，读取失败后保留原请求体并允许重试", async ({ page }) => {
  await setupKuma(page);
  let release!: () => void;
  const response = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/uptime-kuma/template-preset", async (route) => {
    await response;
    await route.fulfill({ status: 503, json: { detail: "预设暂时不可用" } });
  });
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "新增模板" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await dialog.getByLabel("请求体", { exact: true }).fill('{"input":"keep"}');
  await dialog.getByRole("combobox", { name: "接口模式" }).click();
  await page.getByRole("option", { name: "OpenAI Responses", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "保存模板" })).toBeDisabled();
  release();
  await expect(page.locator("[data-sonner-toast]")).toContainText("预设暂时不可用");
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue('{"input":"keep"}');
  await expect(dialog.getByRole("combobox", { name: "接口模式" })).toContainText("自定义请求");
  await expect(dialog.getByRole("button", { name: "保存模板" })).toBeEnabled();
});
