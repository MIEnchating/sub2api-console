import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

for (const resource of [
  { path: "templates", title: "模板", item: "API JSON 模板", apiPath: "templates" },
  {
    path: "status-pages",
    title: "状态页管理",
    item: "服务状态",
    apiPath: "resources/status-pages",
  },
]) {
  test(`${resource.title}读取失败仅显示悬浮提示，刷新成功后恢复列表`, async ({ page }) => {
    await setupKuma(page);
    let failed = true;
    await page.route(`**/api/uptime-kuma/${resource.apiPath}`, async (route) => {
      if (failed) {
        await route.fulfill({ status: 404, json: { detail: "请求失败（404）" } });
      } else {
        await route.fallback();
      }
    });

    await page.goto(`/uptime-kuma/${resource.path}`);

    const messages = page.locator("[data-sonner-toast]");
    await expect(messages).toHaveCount(1);
    await expect(messages).toBeVisible();
    await expect(messages).toContainText("请求失败（404）");
    await expect(page.getByRole("main")).not.toContainText(/读取失败|请求失败|重新验证/);
    await expect(page.getByRole("button", { name: `新增${resource.title}` })).toBeDisabled();
    await page.screenshot({
      path: test.info().outputPath("message-only.png"),
      fullPage: true,
      animations: "disabled",
    });

    failed = false;
    await page.getByRole("button", { name: /刷新/ }).click();

    await expect(page.getByRole("cell", { name: resource.item, exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: `新增${resource.title}` })).toBeEnabled();
  });
}
