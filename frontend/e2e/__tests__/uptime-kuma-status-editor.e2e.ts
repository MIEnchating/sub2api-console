import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";
import {
  statusOptions,
  statusPage,
} from "../../src/features/uptime-kuma/components/__tests__/status-page-fixtures";
import { monitor } from "../../src/features/uptime-kuma/components/__tests__/fixtures";

test("状态页编辑立即在弹窗内加载，列表位置不变且取消后迟到响应不会重开", async ({
  page,
  colorScheme,
}) => {
  await setupKuma(page);
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  let release!: () => void;
  const responseReady = new Promise<void>((resolve) => {
    release = resolve;
  });
  let requested!: () => void;
  const requestStarted = new Promise<void>((resolve) => {
    requested = resolve;
  });
  let finished!: () => void;
  const requestFinished = new Promise<void>((resolve) => {
    finished = resolve;
  });
  await page.route("**/api/uptime-kuma/resources/status-pages/10", async (route) => {
    requested();
    await responseReady;
    await route.fulfill({ json: statusPage });
    finished();
  });
  await page.goto("/uptime-kuma/status-pages");
  const table = page.locator('table[aria-label="状态页管理"]');
  const original = await table.boundingBox();
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  await requestStarted;
  const dialog = page.getByRole("dialog", { name: "编辑状态页管理" });
  await expect(dialog.getByRole("status", { name: "正在读取编辑配置…" })).toBeVisible();
  await expect(dialog.getByText("正在读取编辑配置…", { exact: true })).toBeVisible();
  await expect(dialog.locator('[data-slot="skeleton"]')).toHaveCount(0);
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "取消" })).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "保存", exact: true })).toBeDisabled();
  expect((await table.boundingBox())?.y).toBe(original?.y);
  await page.screenshot({
    path: test.info().outputPath("status-editor-loading.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "取消" }).click();
  release();
  await requestFinished;
  await expect(dialog).toBeHidden();
});

test("状态页可排序监控并单独设置公开地址，添加监控后保留顺序并提交", async ({ page }) => {
  const fixture = await setupKuma(page);
  const longName = "接口名称".repeat(30);
  const options = {
    ...statusOptions,
    monitors: [...statusOptions.monitors, { ...monitor, id: 21, name: longName }],
  };
  await page.route("**/api/uptime-kuma/resources/status-pages", (route) =>
    route.fulfill({ json: options }),
  );
  await page.route("**/api/uptime-kuma/resources/status-pages/10", async (route) => {
    if (route.request().method() !== "GET") return route.fallback();
    await route.fulfill({ json: statusPage });
  });
  await page.goto("/uptime-kuma/status-pages");
  await page.getByRole("button", { name: "编辑", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "编辑状态页管理" });
  await dialog.getByRole("button", { name: "下移监控项 智谱" }).press("Enter");
  await dialog.getByRole("checkbox", { name: /公开智谱的监控地址/ }).check();
  await dialog.getByRole("combobox", { name: "分组 1 监控项" }).click();
  await page.getByRole("option", { name: longName, exact: true }).click();
  await dialog.getByRole("heading", { name: "编辑状态页管理" }).click();
  const order = dialog.getByRole("list", { name: "分组 1 展示顺序" });
  await expect(order.getByRole("listitem").nth(0)).toContainText("备用接口");
  await expect(order.getByRole("listitem").nth(1)).toContainText("智谱");
  await expect(order.getByRole("listitem").nth(2)).toContainText(longName);
  await order.getByRole("listitem").nth(2).scrollIntoViewIfNeeded();
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("status-monitor-order.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    action: "edit",
    revision: "s1",
    association_revision: "a1",
    status_page: {
      groups: [
        {
          id: 11,
          name: "API",
          monitorList: [
            { id: 20, sendUrl: true },
            { id: 19, sendUrl: true, url: "https://example.com/one" },
            { id: 21, sendUrl: false },
          ],
        },
        { id: 12, name: "备用", monitorList: [] },
      ],
    },
  });
});
