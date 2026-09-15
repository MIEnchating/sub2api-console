import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";
import {
  statusOptions,
  statusPage,
} from "../../src/features/uptime-kuma/components/__tests__/status-page-fixtures";

test("组内监控与外层分组独立拖动，取消不关闭弹窗，保存保留稳定 ID 和地址", async ({
  page,
}, testInfo) => {
  const fixture = await setupKuma(page);
  await page.route("**/api/uptime-kuma/resources/status-pages**", async (route) => {
    if (route.request().method() !== "GET") return route.fallback();
    await route.fulfill({
      json: new URL(route.request().url()).pathname.endsWith("/10") ? statusPage : statusOptions,
    });
  });
  await page.goto("/uptime-kuma/status-pages");
  await page
    .getByRole("group", { name: "服务状态 的操作" })
    .getByRole("button", { name: "编辑", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  const monitor = dialog.getByRole("button", { name: "拖动智谱", exact: true });
  await monitor.hover();
  await monitor.focus();
  await monitor.evaluate((node) => node.scrollIntoView({ block: "center" }));
  await page.keyboard.press("Space");
  await expect(monitor).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("目标位置 1", { exact: true })).toBeAttached();
  await page.keyboard.press("ArrowDown");
  await expect(page.getByText("目标位置 2", { exact: true })).toBeAttached();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  await expect(monitor).toHaveAttribute("aria-pressed", "false");
  await expect(
    dialog.getByRole("listitem").filter({
      has: page.getByRole("button", { name: "拖动智谱", exact: true }),
    }),
  ).toHaveCSS("opacity", "1");
  await dialog
    .getByRole("list", { name: "分组 1 展示顺序" })
    .evaluate((node) => node.scrollIntoView({ block: "center" }));
  await dialog.getByRole("button", { name: "拖动备用接口", exact: true }).hover();
  await monitor.hover();
  const from = (await monitor.boundingBox())!;
  const to = (await dialog
    .getByRole("button", { name: "拖动备用接口", exact: true })
    .boundingBox())!;
  await page.mouse.move(from.x + 16, from.y + 16);
  await page.mouse.down();
  await page.mouse.move(to.x + 16, to.y + 16, { steps: 8 });
  await expect(page.getByText("目标位置 2", { exact: true })).toBeAttached();
  await page.mouse.up();
  await expect(dialog.getByRole("button", { name: "下移监控项 智谱", exact: true })).toBeDisabled();
  await expect(dialog.getByLabel("分组 1 名称", { exact: true })).toHaveValue("API");
  const group = dialog.getByRole("button", { name: "拖动展示分组 2", exact: true });
  await group.focus();
  await group.evaluate((node) => node.scrollIntoView({ block: "center" }));
  await page.keyboard.press("Space");
  await expect(group).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("目标位置 2", { exact: true })).toBeAttached();
  await page.keyboard.press("ArrowUp");
  await expect(page.getByText("目标位置 1", { exact: true })).toBeAttached();
  await page.keyboard.press("Space");
  await expect(dialog.getByLabel("分组 1 名称", { exact: true })).toHaveValue("备用");
  await page.screenshot({ path: testInfo.outputPath("status-page-sorted.png") });
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect.poll(() => fixture.writes.length).toBe(1);
  expect(fixture.writes[0].value).toMatchObject({
    status_page: {
      groups: [
        { id: 12, name: "备用", monitorList: [] },
        {
          id: 11,
          name: "API",
          monitorList: [
            { id: 20, sendUrl: true },
            { id: 19, sendUrl: false, url: "https://example.com/one" },
          ],
        },
      ],
    },
  });
});
