import { expect, test, type Route } from "@playwright/test";
import { template } from "../../src/features/account-workbench/__tests__/template-fixture";
import { pageFixtures } from "./fixtures/page-shell";

for (const action of ["编辑配置", "删除模板"] as const) {
  test(`${action}弹窗遇到后台刷新仍保留原版本，冲突后可重新打开最新模板`, async ({
    page,
    colorScheme,
  }) => {
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
      colorScheme,
    );
    const library = { revision: 3, preferred_id: template.id, items: [template] };
    const latest = {
      ...library,
      revision: 4,
      items: [{ ...template, name: "远端更新模板", revision: 2 }],
    };
    const writes: Array<Record<string, unknown>> = [];
    let holdRefresh = false;
    let held: Route | undefined;
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const method = route.request().method();
      if (method !== "GET") {
        if (path !== "/api/account-workbench/templates") {
          await route.fulfill({ status: 503, json: { detail: "隔离测试禁止此写入" } });
          return;
        }
        const body = route.request().postDataJSON() as Record<string, unknown>;
        writes.push(body);
        if (body.revision === 4) await route.fulfill({ json: { ...latest, revision: 5 } });
        else
          await route.fulfill({
            status: 409,
            json: { detail: "工作台资料已变化，请刷新后重试" },
          });
        return;
      }
      if (path === "/api/account-workbench/templates" && holdRefresh) {
        held = route;
        return;
      }
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "模板版本测试" },
        "/api/dictionaries": { items: [] },
        "/api/account-workbench/templates": library,
        "/api/account-workbench/accounts": [template.summary],
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto("/account-workbench?tab=templates");
    const card = page.getByRole("article", { name: template.name });
    await expect(card).toBeVisible();
    holdRefresh = true;
    await page.getByRole("button", { name: "刷新", exact: true }).click();
    await expect.poll(() => !!held).toBe(true);
    await card
      .getByRole("button", { name: action === "编辑配置" ? action : "删除模板 团队模板" })
      .click();
    const dialog = page.getByRole("dialog");
    if (action === "编辑配置")
      await dialog.getByRole("textbox", { name: "模板名称" }).fill("未提交草稿");
    await held!.fulfill({ json: latest });
    await expect(
      page.getByRole("heading", { name: "远端更新模板", includeHidden: true }),
    ).toBeAttached();
    const confirm = dialog.getByRole("button", {
      name: action === "编辑配置" ? "保存模板" : "删除模板",
      exact: true,
    });
    await confirm.click();
    await expect.poll(() => writes.length).toBe(1);
    expect(writes[0]).toMatchObject({ id: template.id, revision: 3 });
    await expect(page.locator("[data-sonner-toast]")).toHaveText("工作台资料已变化，请刷新后重试");
    await expect(dialog).toBeVisible();
    if (action === "编辑配置")
      await expect(dialog.getByRole("textbox", { name: "模板名称" })).toHaveValue("未提交草稿");
    else await expect(dialog).toContainText("确定删除“团队模板”");
    const cancel = dialog.getByRole("button", { name: "取消", exact: true });
    await expect(cancel).toBeEnabled();
    await expect(cancel).toBeInViewport();
    expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
    await page.screenshot({ path: test.info().outputPath("template-conflict.png") });
    await cancel.focus();
    await page.keyboard.press("Enter");
    await expect(dialog).toHaveCount(0);
    await page
      .getByRole("article", { name: "远端更新模板" })
      .getByRole("button", {
        name: action === "编辑配置" ? action : "删除模板 远端更新模板",
      })
      .click();
    if (action === "编辑配置")
      await expect(dialog.getByRole("textbox", { name: "模板名称" })).toHaveValue("远端更新模板");
    else await expect(dialog).toContainText("确定删除“远端更新模板”");
    await confirm.click();
    await expect(dialog).toHaveCount(0);
    expect(writes).toHaveLength(2);
    expect(writes[1]).toMatchObject({ id: template.id, revision: 4 });
  });
}
