import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

test("新增监控选择模板后可设置独立请求模型且不读取模板私密详情", async ({ page }) => {
  const fixture = await setupKuma(page);
  const details: string[] = [];
  page.on("request", (request) => {
    if (/\/api\/uptime-kuma\/templates\/[^/]+$/.test(new URL(request.url()).pathname))
      details.push(request.url());
  });
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "新增监控项", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新增监控项", exact: true });
  await expect(dialog.getByLabel("请求模型", { exact: true })).toHaveCount(0);
  await dialog.getByLabel("监控项名称").fill("自定义模型监控");
  await dialog.getByLabel("监控地址", { exact: true }).fill("https://monitor.example/v1/messages");
  await dialog.getByRole("combobox", { name: "功能模板" }).click();
  await page.getByRole("option", { name: "API JSON 模板", exact: true }).click();
  await dialog.getByLabel("请求模型", { exact: true }).fill("custom-model");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    monitor: { template_id: "a".repeat(48), template_revision: 2, template_model: "custom-model" },
  });
  expect(details).toEqual([]);
});

test("切回手动设置清除模型覆盖，再选模板留空时沿用模板模型", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "新增监控项", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新增监控项", exact: true });
  await dialog.getByLabel("监控项名称").fill("模板默认模型");
  await dialog.getByLabel("监控地址", { exact: true }).fill("https://monitor.example/probe");
  await dialog.getByRole("combobox", { name: "功能模板" }).click();
  await page.getByRole("option", { name: "API JSON 模板", exact: true }).click();
  await dialog.getByLabel("请求模型", { exact: true }).fill("invalid model");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog.getByLabel("请求模型", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(fixture.writes).toHaveLength(0);
  await dialog.getByRole("combobox", { name: "功能模板" }).click();
  await page.getByRole("option", { name: "手动设置", exact: true }).click();
  await expect(dialog.getByLabel("请求模型", { exact: true })).toHaveCount(0);
  await dialog.getByRole("combobox", { name: "功能模板" }).click();
  await page.getByRole("option", { name: "API JSON 模板", exact: true }).click();
  await expect(dialog.getByLabel("请求模型", { exact: true })).toHaveValue("");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({ monitor: { template_model: "" } });
});

test("后台拒绝模板模型覆盖时在模型字段显示原因并保留输入", async ({ page }) => {
  await setupKuma(page);
  await page.route("**/api/tasks/fixture-task", async (route) =>
    route.fulfill({
      json: {
        id: "fixture-task",
        status: "failed",
        progress: 100,
        message: "模板请求体须为 JSON 对象，请先修改模板请求体",
        result: { error_code: "kuma_invalid_template_model" },
      },
    }),
  );
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "新增监控项", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新增监控项", exact: true });
  await dialog.getByLabel("监控项名称").fill("模型校验");
  await dialog.getByLabel("监控地址", { exact: true }).fill("https://monitor.example/probe");
  await dialog.getByRole("combobox", { name: "功能模板" }).click();
  await page.getByRole("option", { name: "API JSON 模板", exact: true }).click();
  await dialog.getByLabel("请求模型", { exact: true }).fill("custom-model");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog.getByLabel("请求模型", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  await expect(dialog.getByLabel("请求模型", { exact: true })).toHaveValue("custom-model");
  await expect(dialog.getByRole("alert")).toContainText("模板请求体须为 JSON 对象");
  await expect(dialog.getByRole("button", { name: "保存监控项" })).toBeEnabled();
});
