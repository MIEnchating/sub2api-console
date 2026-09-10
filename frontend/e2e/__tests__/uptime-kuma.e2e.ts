import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";
import { monitor } from "../../src/features/uptime-kuma/components/__tests__/fixtures";

test("监控表格填满内容区，操作在行内，配置操作固定页头且没有互跳", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma");
  const list = page.getByRole("region", { name: "监控项列表" });
  await expect(list).toBeVisible();
  const content = page.locator('[data-slot="page-content"]');
  const panel = (await list.boundingBox())!;
  const bounds = (await content.boundingBox())!;
  expect(panel.width).toBeGreaterThan(bounds.width - 40);
  expect(panel.height).toBeGreaterThan(300);
  await expect(
    page.locator('[data-slot="page-heading"]').getByRole("link", { name: "接入配置" }),
  ).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const filter = page.getByRole("combobox", { name: "筛选监控状态" });
  await filter.click();
  await page.getByRole("option", { name: "维护中", exact: true }).click();
  await expect(page.getByText("没有匹配的监控项", { exact: true })).toBeVisible();
  await filter.click();
  await page.getByRole("option", { name: "全部状态", exact: true }).click();
  await page.getByRole("button", { name: "查看 智谱主线 详情" }).click();
  await expect(page.getByRole("dialog", { name: "智谱主线" })).toBeVisible();
  await page.keyboard.press("Escape");
  const row = page.getByRole("row", { name: /智谱主线/ });
  const actions = row.getByRole("group", { name: "智谱主线 的操作" });
  await actions.getByRole("button", { name: "删除", exact: true }).click();
  const confirmation = page.getByRole("dialog", { name: "删除监控项" });
  await expect(confirmation).toContainText("ID 19");
  expect(fixture.writes).toHaveLength(0);
  await confirmation.getByRole("button", { name: "删除", exact: true }).click();
  await expect(confirmation).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({ action: "delete", config_revision: 1 });
  await list.locator('[data-slot="table-container"]').evaluate((e) => e.scrollTo(0, 0));
  expect(await content.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("kuma-table.png"), fullPage: true });
  await page.goto("/uptime-kuma/config");
  await expect(page.getByLabel("API 密钥", { exact: true })).toHaveValue("");
  const heading = page.locator('[data-slot="page-heading"]');
  await expect(heading.getByRole("button", { name: "验证并保存" })).toBeInViewport();
  await expect(heading.getByRole("button", { name: "断开接入", exact: true })).toBeInViewport();
  await expect(heading.getByRole("link", { name: "监控管理" })).toHaveCount(0);
  await content.evaluate((e) => e.scrollTo(0, e.scrollHeight));
  await expect(heading.getByRole("button", { name: "验证并保存" })).toBeInViewport();
  await content.evaluate((e) => e.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath("kuma-config.png"), fullPage: true });
});

test("Push 地址按需读取，关闭详情后隐藏，读取失败可以重试", async ({ page }) => {
  await setupKuma(page, [{ ...monitor, id: 21, key: "id:21", type: "push", name: "Push 作业" }]);
  let reads = 0;
  await page.route("**/api/uptime-kuma/monitors/21/push-url?**", async (route) => {
    reads++;
    if (reads === 1) await route.fulfill({ status: 502, json: { detail: "连接暂不可用" } });
    else await route.fulfill({ json: { url: "https://kuma.example/api/push/test-token" } });
  });
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "查看 Push 作业 详情" }).click();
  expect(reads).toBe(0);
  await page.getByRole("button", { name: "显示上报地址" }).click();
  await expect(page.getByText("连接暂不可用", { exact: false })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Push 上报地址" })).toHaveCount(0);
  await page.getByRole("button", { name: "显示上报地址" }).click();
  await expect(page.getByRole("textbox", { name: "Push 上报地址" })).toHaveValue(
    "https://kuma.example/api/push/test-token",
  );
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "查看 Push 作业 详情" }).click();
  await expect(page.getByRole("textbox", { name: "Push 上报地址" })).toHaveCount(0);
  expect(reads).toBe(2);
});

test("分页后搜索回到第一页，行操作支持键盘确认", async ({ page }) => {
  const fixture = await setupKuma(
    page,
    Array.from({ length: 21 }, (_, i) => ({
      ...monitor,
      id: 100 + i,
      key: `id:${100 + i}`,
      name: `备用 ${i}`,
      active: false,
    })),
  );
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "转到下一页" }).click();
  await expect(page.getByRole("button", { name: "查看 备用 20 详情" })).toBeVisible();
  await page.getByRole("textbox", { name: "搜索监控项或地址" }).fill("备用 0");
  await expect(page.getByRole("button", { name: "转到第 1 页" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  const action = page
    .getByRole("group", { name: "备用 0 的操作" })
    .getByRole("button", { name: "恢复" });
  await action.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "恢复监控项" });
  await expect(dialog).toContainText("ID 100");
  await dialog.getByRole("button", { name: "恢复", exact: true }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.path).toBe("/api/uptime-kuma/monitors/100");
});

test("新增 TCP、HTTP 高级请求和调整所属分组提交真实表单值", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "新增监控项" }).click();
  let dialog = page.getByRole("dialog", { name: "新增监控项" });
  await dialog.getByLabel("监控项名称").fill("TCP 服务");
  await dialog.getByRole("combobox", { name: "监控类型" }).click();
  await page.getByRole("option", { name: "TCP 端口", exact: true }).click();
  await dialog.getByLabel("主机名 / IP").fill("api.example");
  await dialog.getByLabel("TCP 端口", { exact: true }).fill("8443");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    action: "create",
    monitor: {
      type: "port",
      options: { hostname: "api.example", port: 8443, notification_ids: [] },
    },
  });
  await page
    .getByRole("group", { name: "智谱主线 的操作" })
    .getByRole("button", { name: "编辑" })
    .click();
  dialog = page.getByRole("dialog", { name: "编辑监控项" });
  await dialog.getByRole("combobox", { name: "所属分组" }).click();
  await page.getByRole("option", { name: "核心分组", exact: true }).click();
  await dialog.getByRole("combobox", { name: "请求方法" }).click();
  await page.getByRole("option", { name: "POST", exact: true }).click();
  await dialog.getByLabel("请求头（JSON，留空保留）").fill('{"Content-Type":"application/json"}');
  await dialog.getByLabel("请求体（留空保留）").fill('{"probe":true}');
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[1]?.value).toMatchObject({
    action: "edit",
    monitor: { parent: 7, options: { method: "POST", body: '{"probe":true}' } },
  });
});
