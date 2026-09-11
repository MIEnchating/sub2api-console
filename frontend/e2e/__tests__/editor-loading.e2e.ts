import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

for (const scenario of [
  {
    path: "templates",
    endpoint: `templates/${"a".repeat(48)}`,
    label: "正在读取模板内容…",
    field: "模板名称",
  },
]) {
  test(`${scenario.path} 编辑读取失败可重试，加载轻量且可关闭`, async ({ page, colorScheme }) => {
    await setupKuma(page);
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
      colorScheme,
    );
    let release!: () => void;
    const ready = new Promise<void>((resolve) => {
      release = resolve;
    });
    let fail = true;
    await page.route(`**/api/uptime-kuma/${scenario.endpoint}`, async (route) => {
      if (fail) {
        await ready;
        await route.fulfill({ status: 502, json: { detail: "编辑配置暂不可用，请重试" } });
      } else await route.fallback();
    });
    await page.goto(`/uptime-kuma/${scenario.path}`);
    await page.getByRole("button", { name: "编辑", exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("status", { name: scenario.label })).toBeVisible();
    await expect(dialog.getByText(scenario.label, { exact: true })).toBeInViewport();
    await expect(dialog.locator('[data-slot="skeleton"]')).toHaveCount(0);
    await expect(dialog.getByRole("button", { name: "取消" })).toBeInViewport();
    await expect(dialog.getByRole("button", { name: /^保存/ })).toBeDisabled();
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    await page.screenshot({
      path: test.info().outputPath(`${scenario.path}-loading.png`),
      animations: "disabled",
    });
    release();
    await expect(dialog.getByRole("button", { name: "重新读取" })).toBeVisible();
    await expect(dialog.getByRole("button", { name: /^保存/ })).toBeDisabled();
    fail = false;
    await dialog.getByRole("button", { name: "重新读取" }).click();
    await expect(dialog.getByLabel(scenario.field, { exact: true })).toBeVisible();
    await expect(dialog.getByRole("status", { name: scenario.label })).toHaveCount(0);
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
  });
}
