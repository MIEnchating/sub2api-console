import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

for (const viewport of [
  { width: 1440, height: 900 },
  { width: 390, height: 664 },
  { width: 320, height: 568 },
]) {
  for (const kind of ["monitor", "resource", "config", "confirmation"] as const) {
    test(`${viewport.width}px ${kind} 创建等待、任务取消与完成刷新不会重复提交`, async ({
      page,
    }) => {
      await page.setViewportSize(viewport);
      await page.emulateMedia({ colorScheme: viewport.width < 640 ? "dark" : "light" });
      await setupKuma(page);
      await page.route("**/api/dictionaries**", (route) => route.fulfill({ json: { items: [] } }));
      let releaseCreate!: () => void;
      let releaseCancel!: () => void;
      const created = new Promise<void>((resolve) => {
        releaseCreate = resolve;
      });
      const accepted = new Promise<void>((resolve) => {
        releaseCancel = resolve;
      });
      let cancelled = false;
      let writes = 0;
      let reads = 0;
      let cancellations = 0;
      const task = {
        id: "cancel-fixture",
        status: "running",
        progress: 0,
        message: "正在验证或更新 Uptime Kuma",
        result: {},
      };
      await page.route("**/api/uptime-kuma/**", async (route) => {
        if (["PUT", "POST"].includes(route.request().method())) {
          writes++;
          await created;
          await route.fulfill({ json: task });
        } else {
          reads++;
          await route.fallback();
        }
      });
      await page.route("**/api/tasks/cancel-fixture", async (route) => {
        if (route.request().method() === "DELETE") {
          cancellations++;
          await accepted;
          cancelled = true;
          await route.fulfill({ json: { cancelled: true } });
        } else {
          await route.fulfill({
            json: {
              ...task,
              status: cancelled ? "cancelled" : "running",
              message: cancelled ? "任务已手动取消" : task.message,
            },
          });
        }
      });
      const errors: Error[] = [];
      page.on("pageerror", (error) => errors.push(error));
      let path = "/uptime-kuma";
      if (kind === "config") path = "/uptime-kuma/config";
      if (kind === "resource") path = "/uptime-kuma/status-pages";
      await page.goto(path);
      if (kind === "monitor") {
        await page.getByRole("button", { name: "新增监控项", exact: true }).click();
        await page.getByLabel("监控项名称").fill("取消测试");
        await page
          .getByLabel("监控地址", { exact: true })
          .fill("https://monitor.example.test/health");
      } else if (kind === "resource") {
        await page.getByRole("button", { name: "新增状态页管理" }).click();
        await page.getByLabel("状态页标题", { exact: true }).fill("取消测试");
        await page.getByLabel("访问路径", { exact: true }).fill("cancel-test");
      } else if (kind === "confirmation") {
        await page
          .getByRole("group", { name: "智谱主线 的操作" })
          .getByRole("button", { name: "暂停", exact: true })
          .click();
      }
      const scope =
        kind === "config"
          ? page.getByRole("tabpanel", { name: "监控平台", exact: true })
          : page.getByRole("dialog");
      const labels = {
        monitor: "保存监控项",
        resource: "保存",
        config: "验证并保存",
        confirmation: "暂停",
      };
      await scope.getByRole("button", { name: labels[kind], exact: true }).click();
      await expect(scope.getByRole("status", { name: "正在创建 Uptime Kuma 任务" })).toBeVisible();
      await expect(scope.getByRole("progressbar")).toHaveCount(0);
      await expect(scope.getByRole("button", { name: "取消任务" })).toHaveCount(0);
      releaseCreate();
      const cancel = scope.getByRole("button", { name: "取消任务" });
      await expect(cancel).toBeEnabled();
      await expect(scope.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "0");
      await cancel.scrollIntoViewIfNeeded();
      await expect(cancel).toBeInViewport({ ratio: 1 });
      expect(await scope.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
        true,
      );
      const readsBefore = reads;
      await cancel.click();
      await expect(scope.getByRole("button", { name: "取消中" })).toBeDisabled();
      if (kind === "monitor") await expect(scope.getByLabel("监控项名称")).toBeDisabled();
      if (kind === "resource")
        await expect(scope.getByLabel("状态页标题", { exact: true })).toBeDisabled();
      if (kind === "config")
        await expect(scope.getByLabel("API 密钥", { exact: true })).toBeDisabled();
      if (viewport.width === 320)
        await page.screenshot({ path: test.info().outputPath(`${kind}-cancelling.png`) });
      releaseCancel();
      const notice = page
        .locator("[data-sonner-toast]")
        .filter({ hasText: "Uptime Kuma 任务已取消" });
      await expect(notice).toHaveCount(1);
      await expect(notice).toContainText("不会自动撤销");
      await expect(notice).toContainText("核对远端状态");
      if (kind !== "config") await expect(scope).toBeHidden();
      else await expect(scope.getByRole("button", { name: "验证并保存" })).toBeEnabled();
      await expect(page.getByRole("progressbar")).toHaveCount(0);
      await expect(page.getByRole("status", { name: "正在创建 Uptime Kuma 任务" })).toHaveCount(0);
      await expect.poll(() => reads).toBeGreaterThan(readsBefore);
      expect(writes).toBe(1);
      expect(cancellations).toBe(1);
      expect(errors).toEqual([]);
    });
  }
}
