import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

for (const scenario of [
  { path: "/uptime-kuma", trigger: "新增监控项", title: "新增监控项" },
  { path: "/uptime-kuma/templates", trigger: "新增模板", title: "新增功能模板" },
  { path: "/uptime-kuma/status-pages", trigger: "新增状态页管理", title: "新增状态页管理" },
]) {
  test(`${scenario.title}在短屏中鼠标位于内容中央时可滚动且操作区保持可见`, async ({ page }) => {
    await page.setViewportSize({ width: test.info().project.use.viewport!.width, height: 380 });
    await setupKuma(page);
    await page.goto(scenario.path);
    await page.getByRole("button", { name: scenario.trigger, exact: true }).click();
    const dialog = page.getByRole("dialog", { name: scenario.title, exact: true });
    const body = dialog.locator('[data-slot="dialog-body"]');
    expect(await body.evaluate((element) => element.scrollHeight)).toBeGreaterThan(
      await body.evaluate((element) => element.clientHeight),
    );
    const dialogBounds = (await dialog.boundingBox())!;
    await page.mouse.move(
      dialogBounds.x + dialogBounds.width / 2,
      dialogBounds.y + dialogBounds.height / 2,
    );
    await page.mouse.wheel(0, 400);
    await expect.poll(() => body.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeInViewport();
    await expect(dialog.getByRole("heading", { name: scenario.title })).toBeInViewport();
    expect(
      await dialog.evaluate((element) => element.scrollHeight - element.clientHeight),
    ).toBeLessThanOrEqual(1);
    await page.mouse.wheel(0, -2000);
    await expect.poll(() => body.evaluate((element) => element.scrollTop)).toBe(0);
  });
}
