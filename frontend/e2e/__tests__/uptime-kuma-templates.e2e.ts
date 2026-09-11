import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

test("功能模板支持新增、请求回显编辑和删除确认，旧导航已移除", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma/templates");
  await expect(page.getByRole("table", { name: "功能模板" })).toBeVisible();
  await expect(page.getByRole("link", { name: "通知渠道", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "维护计划", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "新增模板" }).click();
  let dialog = page.getByRole("dialog", { name: "新增功能模板" });
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await dialog.getByLabel("模板名称").fill("JSON 检测");
  await dialog.getByLabel("请求头（JSON）").fill("[]");
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog.getByLabel("请求头（JSON）")).toHaveAttribute("aria-invalid", "true");
  expect(fixture.writes).toHaveLength(0);
  await dialog.getByLabel("请求头（JSON）").fill('{"X-Key":"test-header"}');
  await dialog.getByLabel("请求体", { exact: true }).fill('{"model":"test","messages":[]}');
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    name: "JSON 检测",
    method: "POST",
    headers: '{"X-Key":"test-header"}',
    body: '{"model":"test","messages":[]}',
    auth_method: "none",
    auth_password: "",
  });
  await page
    .getByRole("group", { name: "API JSON 模板 的操作" })
    .getByRole("button", { name: "编辑" })
    .click();
  dialog = page.getByRole("dialog", { name: "编辑功能模板" });
  await dialog.getByRole("button", { name: "查看请求体" }).click();
  await expect(dialog.getByLabel("请求头（JSON）")).toHaveValue('{"X-Key":"saved-header"}');
  await expect(dialog.getByLabel("请求体", { exact: true })).toHaveValue(
    '{"model":"saved-model","messages":[]}',
  );
  await expect(dialog.getByRole("combobox", { name: "鉴权方式", exact: true })).toHaveCount(0);
  await dialog.getByLabel("模板名称").fill("重命名");
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[1]?.value).toMatchObject({
    revision: 2,
    name: "重命名",
    headers: '{"X-Key":"saved-header"}',
    body: '{"model":"saved-model","messages":[]}',
    auth_password: "",
  });
  await page
    .getByRole("group", { name: "API JSON 模板 的操作" })
    .getByRole("button", { name: "删除" })
    .click();
  dialog = page.getByRole("dialog", { name: "删除功能模板" });
  expect(fixture.writes).toHaveLength(2);
  await dialog.getByRole("button", { name: "删除模板" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[2]?.value).toEqual({ revision: 2 });
  await page.screenshot({ path: test.info().outputPath("templates.png"), fullPage: true });
});

test("HTTP 监控选择模板时提交稳定 ID 和版本，TCP 不使用请求模板", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma");
  await page
    .getByRole("group", { name: "智谱主线 的操作" })
    .getByRole("button", { name: "编辑" })
    .click();
  let dialog = page.getByRole("dialog", { name: "编辑监控项" });
  await dialog.getByLabel("请求头（JSON，留空保留）").fill("[]");
  await dialog.getByRole("combobox", { name: "功能模板" }).click();
  await page.getByRole("option", { name: "API JSON 模板", exact: true }).click();
  await expect(dialog.getByRole("combobox", { name: "HTTP 鉴权方式" })).toContainText("无鉴权");
  await expect(dialog.getByLabel("请求头（JSON，留空保留）")).toHaveCount(0);
  await expect(dialog.getByRole("combobox", { name: "通知渠道" })).toHaveCount(0);
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    monitor: {
      template_id: "a".repeat(48),
      template_revision: 2,
      template_auth_override: false,
      options: { headers: "", auth_password: "" },
    },
  });
  await page.getByRole("button", { name: "新增监控项" }).click();
  dialog = page.getByRole("dialog", { name: "新增监控项" });
  await dialog.getByRole("combobox", { name: "监控类型" }).click();
  await page.getByRole("option", { name: "TCP 端口", exact: true }).click();
  await expect(dialog.getByRole("combobox", { name: "功能模板" })).toContainText("手动设置");
});

test("状态页管理保留编辑和展示分组排序", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma/status-pages");
  await page
    .getByRole("group", { name: "服务状态 的操作" })
    .getByRole("button", { name: "编辑" })
    .click();
  const dialog = page.getByRole("dialog", { name: "编辑状态页管理" });
  await dialog.getByRole("button", { name: "添加展示分组" }).click();
  await dialog.getByLabel("分组 2 名称").fill("首要服务");
  await dialog.getByRole("button", { name: "上移展示分组 2" }).click();
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    status_page: { groups: [{ name: "首要服务" }, { id: 11, name: "API" }] },
  });
});
