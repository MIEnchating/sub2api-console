import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

const detailPath = `**/api/uptime-kuma/templates/${"a".repeat(48)}`;
const detail = {
  id: "a".repeat(48),
  revision: 3,
  name: "API JSON 模板",
  method: "POST",
  auth_method: "bearer",
  headers_configured: true,
  body_configured: true,
  auth_configured: true,
  headers: '{"X-Key":"stored-header"}',
  body: '{"input":"stored-body"}',
};

test("打开编辑时等待详情回显，修改后提交最新版本和请求内容", async ({ page }) => {
  const fixture = await setupKuma(page);
  let release!: () => void;
  const response = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(detailPath, async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback();
      return;
    }
    await response;
    await route.fulfill({ json: detail });
  });
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑功能模板" });
  await expect(dialog.getByRole("status", { name: "正在读取模板内容…" })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "保存模板" })).toBeDisabled();
  release();
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await expect(dialog.getByLabel("请求头（JSON）")).toHaveValue(detail.headers);
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(detail.body);
  await expect(dialog.getByRole("combobox", { name: "鉴权方式", exact: true })).toHaveCount(0);
  await dialog.getByLabel("请求体", { exact: true }).fill('{"input":"updated"}');
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    revision: 3,
    headers: detail.headers,
    body: '{"input":"updated"}',
    auth_password: "",
    clear_headers: false,
    clear_body: false,
  });
});

test("清空回显文本时提交明确清空标记", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑功能模板" });
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).not.toHaveValue("");
  await dialog.getByLabel("请求体", { exact: true }).fill("");
  await dialog.getByLabel("请求头（JSON）").fill("");
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    headers: "",
    body: "",
    clear_headers: true,
    clear_body: true,
  });
});

test("详情读取失败时禁止保存，重试成功后恢复回显", async ({ page }) => {
  const fixture = await setupKuma(page);
  let failed = true;
  await page.route(detailPath, async (route) => {
    if (failed) await route.fulfill({ status: 503, json: { detail: "模板详情暂时无法读取" } });
    else await route.fulfill({ json: detail });
  });
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑功能模板" });
  await expect(dialog.getByRole("button", { name: "重新读取" })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "保存模板" })).toBeDisabled();
  await expect(page.locator("[data-sonner-toast]")).toHaveCount(1);
  expect(fixture.writes).toHaveLength(0);
  failed = false;
  await dialog.getByRole("button", { name: "重新读取" }).click();
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(detail.body);
  await expect(dialog.getByRole("button", { name: "保存模板" })).toBeEnabled();
});

test("关闭编辑后再次打开重新读取内容，不保留上次未保存的请求", async ({ page }) => {
  await setupKuma(page);
  let body = "first saved body";
  await page.route(detailPath, async (route) => route.fulfill({ json: { ...detail, body } }));
  await page.goto("/uptime-kuma/templates");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  let dialog = page.getByRole("dialog", { name: "编辑功能模板" });
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(body);
  await dialog.getByLabel("请求体", { exact: true }).fill("unsaved body");
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).toBeHidden();
  body = "latest saved body";
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  dialog = page.getByRole("dialog", { name: "编辑功能模板" });
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(body);
});
