import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

for (const width of [1440, 320]) {
  for (const scenario of ["config-first", "monitors-first", "config-refresh"] as const) {
    test(`${width}px ${scenario} 读取失败可恢复且不允许使用失效配置编辑`, async ({ page }) => {
      await page.setViewportSize({ width, height: width === 320 ? 568 : 900 });
      await page.emulateMedia({ colorScheme: width === 320 ? "dark" : "light" });
      const fixture = await setupKuma(page);
      await page.route("**/api/dictionaries**", (route) => route.fulfill({ json: { items: [] } }));
      let failed = scenario !== "config-refresh";
      const endpoint = scenario === "monitors-first" ? "monitors" : "config";
      await page.route(`**/api/uptime-kuma/${endpoint}`, async (route) => {
        if (failed) {
          await route.fulfill({ status: 503, json: { detail: "隔离读取失败" } });
        } else {
          await route.fallback();
        }
      });
      const errors: Error[] = [];
      page.on("pageerror", (error) => errors.push(error));
      await page.goto("/uptime-kuma");
      if (scenario === "config-refresh") {
        await expect(page.getByRole("button", { name: "新增监控项", exact: true })).toBeEnabled();
        failed = true;
        await page.getByRole("button", { name: "刷新监控项" }).click();
      }
      const notice = page.locator("[data-sonner-toast]").filter({ hasText: "隔离读取失败" });
      await expect(notice).toHaveCount(1);
      if (scenario === "config-refresh") {
        await expect(
          page.getByRole("button", { name: "查看 智谱主线 详情", exact: true }),
        ).toBeVisible();
        await expect(page.getByRole("button", { name: "新增监控项", exact: true })).toBeDisabled();
        const actions = page.getByRole("group", { name: "智谱主线 的操作" });
        await expect(actions.getByRole("button", { name: "编辑", exact: true })).toBeDisabled();
        await expect(actions.getByRole("button", { name: "暂停", exact: true })).toBeDisabled();
      } else {
        const retry = page.getByRole("button", { name: "重新读取", exact: true });
        await expect(retry).toBeEnabled();
        await expect(retry).toBeInViewport({ ratio: 1 });
        await expect(page.getByRole("progressbar")).toHaveCount(0);
      }
      if (width === 320) {
        await page.screenshot({ path: test.info().outputPath(`${scenario}.png`) });
      }
      failed = false;
      const retryLabel = scenario === "config-refresh" ? "刷新监控项" : "重新读取";
      await page.getByRole("button", { name: retryLabel, exact: true }).click();
      await expect(page.getByRole("button", { name: "新增监控项", exact: true })).toBeEnabled();
      await expect(
        page.getByRole("button", { name: "查看 智谱主线 详情", exact: true }),
      ).toBeVisible();
      await expect(page.getByRole("button", { name: "重新读取", exact: true })).toHaveCount(0);
      expect(fixture.writes).toEqual([]);
      expect(errors).toEqual([]);
    });
  }
}
