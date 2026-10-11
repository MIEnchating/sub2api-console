import { expect, test } from "@playwright/test";
import { pageFixtures, workspace } from "./fixtures/page-shell";

for (const viewport of [
  { width: 1440, height: 790 },
  { width: 390, height: 664 },
]) {
  test(`添加运营任务在 ${viewport.width} 宽度下保持紧凑且可滚动操作`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    const groups = Array.from({ length: 12 }, (_, index) => ({
      id: String(index + 1),
      name: `生产账号分组 ${index + 1}`,
      account_count: 19,
    }));
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "布局测试" },
        "/api/newapi": {
          ...workspace,
          bindings: groups.map((group) => ({
            platform_id: "layout",
            sub2api_group_id: group.id,
            newapi_group_id: group.id,
            newapi_group_name: group.name,
            sync_ratio: true,
          })),
        },
        "/api/groups": groups,
        "/api/newapi/platforms/layout/channels": {
          items: groups.map((group) => ({
            id: group.id,
            name: `模型渠道 ${group.id}`,
            type: 1,
            status: 1,
            models: [],
            groups: [],
            version: "a".repeat(64),
          })),
          total: groups.length,
        },
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      else await route.fulfill({ json: fixtures[path] ?? [] });
    });
    await page.goto("/newapi/account-model-automation");
    await page.getByRole("button", { name: "添加任务", exact: true }).click();
    const editor = page.getByRole("dialog", { name: "添加运营任务" });
    await expect(editor.getByRole("checkbox", { name: /生产账号分组 1（/ })).toBeVisible();
    const body = editor.locator("[data-slot=dialog-body]");
    if (viewport.width > 1000) {
      const settings = await editor.getByRole("region", { name: "任务设置" }).boundingBox();
      expect(settings!.height).toBeLessThan(150);
      const name = await editor.getByRole("textbox", { name: "运营任务名称" }).boundingBox();
      expect(name!.y - settings!.y).toBeLessThan(40);
      const strategy = await editor.getByRole("region", { name: "同步策略" }).boundingBox();
      const area = await body.boundingBox();
      expect(strategy!.y + strategy!.height).toBeLessThanOrEqual(area!.y + area!.height + 1);
      await expect(editor.getByRole("checkbox", { name: "上架所选模型" })).toBeDisabled();
    }
    expect(await editor.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await editor.getByRole("textbox", { name: "运营任务名称" }).fill("生产模型同步");
    await editor.getByRole("checkbox", { name: /生产账号分组 1（/ }).check();
    await expect(editor.getByRole("button", { name: "保存任务", exact: true })).toBeEnabled();
    await editor.getByRole("radio", { name: "指定时段" }).check();
    await editor.getByRole("button", { name: "添加时间" }).click();
    await editor.getByLabel("每天运行时间 2", { exact: true }).fill("18:30");
    await expect(editor.getByRole("button", { name: "保存任务", exact: true })).toBeEnabled();
    if (viewport.width > 1000) {
      const settings = await editor.getByRole("region", { name: "任务设置" }).boundingBox();
      expect(settings!.height).toBeLessThan(170);
    }
    await body.evaluate((element) => {
      element.scrollTop = 0;
    });
    await page.screenshot({ path: testInfo.outputPath("editor.png") });
  });
}
