import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";
import { config } from "../../src/features/uptime-kuma/components/__tests__/fixtures";

test.beforeEach(async ({ page, viewport, colorScheme }) => {
  if (viewport!.width < 640) await page.setViewportSize({ width: 320, height: 568 });
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

for (const scenario of ["monitor-edit", "monitor-confirm", "resource-edit", "resource-confirm"]) {
  test(`${scenario} 已打开窗口遇到配置更新时阻止跨实例提交，关闭可重新操作`, async ({ page }) => {
    const fixture = await setupKuma(page);
    const monitorPage = scenario.startsWith("monitor");
    const confirmation = scenario.endsWith("confirm");
    await page.goto(monitorPage ? "/uptime-kuma" : "/uptime-kuma/status-pages");
    const actions = page.getByRole("group", {
      name: monitorPage ? "智谱主线 的操作" : "服务状态 的操作",
    });
    await expect(actions.getByRole("button", { name: "编辑", exact: true })).toBeEnabled();
    const changed = { ...config, revision: 2, base_url: "https://changed.example.test" };
    let releaseConfig!: () => void;
    const configReady = new Promise<void>((resolve) => {
      releaseConfig = resolve;
    });
    await page.route("**/api/uptime-kuma/config", async (route) => {
      await configReady;
      await route.fulfill({ json: changed });
    });
    await page
      .getByRole("button", { name: monitorPage ? "刷新监控项" : "刷新", exact: true })
      .click();
    await actions
      .getByRole("button", { name: confirmation ? "删除" : "编辑", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    const submit = dialog.getByRole("button", {
      name: confirmation ? "删除" : /^保存/,
      exact: confirmation,
    });
    await expect(submit).toBeEnabled();
    releaseConfig();
    const link = page.locator(`a[href^="${changed.base_url}"]`);
    await expect(link).toHaveCount(1);
    await submit.focus();
    await page.keyboard.press("Enter");
    await expect(
      page.getByText("监控平台配置已更改，请关闭窗口并重新打开后操作", { exact: true }),
    ).toHaveCount(1);
    expect(fixture.writes).toEqual([]);
    await expect(dialog).toBeVisible();
    await page.screenshot({ path: test.info().outputPath("scope-rejected.png") });
    await dialog.getByRole("button", { name: "取消", exact: true }).click();
    await expect(dialog).toHaveCount(0);
  });
}

for (const phase of ["requesting", "distributing"]) {
  test(`${phase} 模板下发可通过键盘取消，保留逐项结果且不重提`, async ({ page }) => {
    await setupKuma(page);
    let submitted = 0;
    let cancelled = false;
    await page.route("**/api/uptime-kuma/templates/*/distribution**", (route) => {
      if (route.request().method() === "POST") {
        submitted++;
        return route.fulfill({
          json: { id: "distribution", status: "queued", progress: 0, result: { phase: "queued" } },
        });
      }
      return route.fulfill({
        json: {
          revision: 2,
          scope: "b".repeat(64),
          items: [{ monitor_id: 19, name: "智谱主线", template_revision: 1, reason: "" }],
        },
      });
    });
    await page.route("**/api/tasks/distribution", (route) => {
      if (route.request().method() === "DELETE") {
        cancelled = true;
        return route.fulfill({ json: { cancelled: true } });
      }
      return route.fulfill({
        json: {
          id: "distribution",
          status: cancelled ? "cancelled" : "running",
          progress: 0,
          message: "正在连接监控平台",
          result: {
            phase,
            remote_write: false,
            items: [{ monitor_id: 19, name: "智谱主线", status: "not_run", message: "尚未下发" }],
          },
        },
      });
    });
    await page.goto("/uptime-kuma/templates");
    await page.getByRole("button", { name: "参数下发", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "参数下发", exact: true });
    await dialog.getByRole("button", { name: "确认下发", exact: true }).click();
    const cancel = dialog.getByRole("button", { name: "取消任务", exact: true });
    await expect(cancel).toBeEnabled();
    await cancel.focus();
    await page.keyboard.press("Enter");
    await expect(dialog.getByRole("list", { name: "参数下发结果" })).toContainText("未执行");
    await expect(page.locator('[data-sonner-toast][data-type="error"]')).toHaveCount(0);
    await expect(
      page.getByText("Uptime Kuma 任务已取消，未执行远端操作", { exact: true }),
    ).toHaveCount(1);
    expect(submitted).toBe(1);
    await expect(dialog.getByRole("button", { name: "确认下发" })).toHaveCount(0);
    await page.screenshot({ path: test.info().outputPath("distribution-cancelled.png") });
    await dialog.getByRole("button", { name: "关闭", exact: true }).first().click();
    await expect(dialog).toHaveCount(0);
  });
}
