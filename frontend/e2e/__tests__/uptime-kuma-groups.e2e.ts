import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";
import { monitor } from "../../src/features/uptime-kuma/components/__tests__/fixtures";

test("嵌套分组可用键盘折叠，搜索和筛选保留父组，编辑后保留分组 ID", async ({ page }) => {
  const group = { ...monitor, id: 31, key: "id:31", name: "openai", type: "group" };
  const nested = { ...group, id: 32, key: "id:32", name: "生产", parent: 31 };
  const child = { ...monitor, id: 33, key: "id:33", name: "codex", parent: 32 };
  const fixture = await setupKuma(page, [child, nested, group]);
  await page.goto("/uptime-kuma");
  const row = page.getByRole("row", { name: /查看 codex 详情/ });
  await expect(row.getByRole("cell", { name: "openai / 生产", exact: true })).toBeVisible();
  const parentName = (await page.getByRole("button", { name: "查看 openai 详情" }).boundingBox())!;
  const childName = (await page.getByRole("button", { name: "查看 codex 详情" }).boundingBox())!;
  expect(childName.x).toBeGreaterThan(parentName.x);
  const collapse = page.getByRole("button", { name: "收起分组 openai" });
  await collapse.focus();
  await page.keyboard.press("Enter");
  await expect(row).toHaveCount(0);
  await expect(page.getByRole("button", { name: "展开分组 openai" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  const search = page.getByRole("textbox", { name: "搜索监控项或地址" });
  await search.fill("codex");
  await expect(page.getByRole("button", { name: "查看 openai 详情" })).toBeVisible();
  await expect(row).toBeVisible();
  await search.fill("");
  const filter = page.getByRole("combobox", { name: "筛选监控分组" });
  await filter.click();
  await page.getByRole("option", { name: "openai", exact: true }).click();
  await expect(row).toBeVisible();
  await expect(page.getByRole("button", { name: "查看 智谱主线 详情" })).toHaveCount(0);
  await row.getByRole("button", { name: "编辑", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑监控项" });
  await expect(dialog.getByRole("combobox", { name: "所属分组" })).toContainText("生产");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]).toMatchObject({
    path: "/api/uptime-kuma/monitors/33",
    value: { monitor: { parent: 32 } },
  });
  await page.screenshot({ path: test.info().outputPath("monitor-groups.png") });
});

test("分组子项跨页时仍显示所属分组，不会成为无分组监控", async ({ page }) => {
  const group = { ...monitor, id: 31, key: "id:31", name: "openai", type: "group" };
  const children = Array.from({ length: 22 }, (_, i) => ({
    ...monitor,
    id: 100 + i,
    key: `id:${100 + i}`,
    name: `服务 ${i}`,
    parent: 31,
  }));
  await setupKuma(page, [group, ...children]);
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "转到下一页" }).click();
  const row = page.getByRole("row", { name: /查看 服务 21 详情/ });
  await expect(row).toBeVisible();
  await expect(row.getByRole("cell", { name: "openai", exact: true })).toBeVisible();
});
