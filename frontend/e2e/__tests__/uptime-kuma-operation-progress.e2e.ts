import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

for (const kind of ["monitor", "status-page"] as const) {
  for (const action of ["create", "edit"] as const) {
    test(`${kind} ${action} 任务进度位于弹窗底部，成功后关闭且不改变表格布局`, async ({ page }) => {
      await setupKuma(page);
      let complete = false;
      const task = {
        id: "progress-fixture",
        status: "running",
        progress: 45,
        message: "正在同步配置",
        result: {},
      };
      await page.route("**/api/uptime-kuma/**", async (route) => {
        if (["POST", "PUT"].includes(route.request().method())) await route.fulfill({ json: task });
        else await route.fallback();
      });
      await page.route("**/api/tasks/progress-fixture", async (route) =>
        route.fulfill({ json: { ...task, status: complete ? "succeeded" : "running" } }),
      );
      const monitor = kind === "monitor";
      await page.goto(monitor ? "/uptime-kuma" : "/uptime-kuma/status-pages");
      const table = page.getByRole("table", { includeHidden: true }).first();
      await expect(table).toBeVisible();
      const before = await table.boundingBox();
      if (action === "edit") {
        await page
          .getByRole("group", { name: monitor ? "智谱主线 的操作" : "服务状态 的操作" })
          .getByRole("button", { name: "编辑", exact: true })
          .click();
      } else {
        await page
          .getByRole("button", { name: monitor ? "新增监控项" : "新增状态页管理", exact: true })
          .click();
      }
      const dialog = page.getByRole("dialog");
      if (action === "create") {
        if (monitor) {
          await dialog.getByLabel("监控项名称").fill("进度测试");
          await dialog.getByLabel("监控地址", { exact: true }).fill("https://monitor.example");
        } else {
          await dialog.getByLabel("状态页标题", { exact: true }).fill("进度测试");
          await dialog.getByLabel("访问路径", { exact: true }).fill("progress");
        }
      }
      await dialog
        .getByRole("button", { name: monitor ? "保存监控项" : "保存", exact: true })
        .click();
      await expect(dialog.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "45");
      const footer = dialog.locator('[data-slot="dialog-footer"]');
      await expect(footer.getByRole("progressbar")).toBeVisible();
      await expect(page.getByRole("progressbar", { includeHidden: true })).toHaveCount(1);
      await expect(footer.getByRole("button", { name: "正在保存…" })).toBeDisabled();
      const bounds = await footer.boundingBox();
      expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(page.viewportSize()!.height);
      const after = await table.boundingBox();
      expect(after!.y).toBe(before!.y);
      complete = true;
      await expect(dialog).toBeHidden();
      await expect(page.getByRole("progressbar")).toHaveCount(0);
    });
  }
}

test("保存任务失败后隐藏弹窗进度，保留输入并恢复保存按钮", async ({ page }) => {
  await setupKuma(page);
  let failed = false;
  const task = {
    id: "failed-fixture",
    status: "running",
    progress: 30,
    message: "正在同步配置",
    result: {},
  };
  await page.route("**/api/uptime-kuma/monitors/19", async (route) => {
    if (route.request().method() === "POST") await route.fulfill({ json: task });
    else await route.fallback();
  });
  await page.route("**/api/tasks/failed-fixture", async (route) =>
    route.fulfill({
      json: {
        ...task,
        status: failed ? "failed" : "running",
        message: failed ? "远端保存失败" : task.message,
      },
    }),
  );
  await page.goto("/uptime-kuma");
  await page
    .getByRole("group", { name: "智谱主线 的操作" })
    .getByRole("button", { name: "编辑", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "编辑监控项" });
  await dialog.getByLabel("监控项名称").fill("保留编辑内容");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog.getByRole("progressbar")).toBeVisible();
  failed = true;
  await expect(dialog.getByRole("progressbar")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "保存监控项" })).toBeEnabled();
  await expect(dialog.getByLabel("监控项名称")).toHaveValue("保留编辑内容");
  await expect(page.locator("[data-sonner-toast]")).toContainText("远端保存失败");
});
