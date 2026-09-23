import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

for (const tab of ["账号检测", "前置检测", "终端续接检测"]) {
  test(`${tab} 卡片支持熔断和恢复且窄屏操作区不溢出`, async ({ page }) => {
    let current = { ...account, platform: "openai" };
    const actions: string[] = [];
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/accounts/41/control") {
        const action = route.request().postDataJSON().action as string;
        actions.push(action);
        current = {
          ...current,
          health: action === "fuse" ? "fused" : "healthy",
          schedulable: action !== "fuse",
        };
        await route.fulfill({
          json: {
            id: `control-${actions.length}`,
            skill: "console",
            operation: "account-control",
            status: "succeeded",
            progress: 100,
            message: "账号处置完成",
            result: {},
            created_at: "2026-09-22T00:00:00Z",
            updated_at: "2026-09-22T00:00:00Z",
          },
        });
        return;
      }
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "隔离测试" },
        "/api/accounts": [current],
        "/api/accounts/41": current,
        "/api/model-checks/animation-schedules": [],
        "/api/model-checks/animations": [],
        "/api/model-checks/terminal-continuity": [],
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto("/animation-check");
    await page.getByRole("tab", { name: tab, exact: true }).click();
    const panel = page.getByRole("tabpanel", { name: tab, exact: true });
    const card = panel.getByRole("article");
    await card.getByRole("button", { name: "手动熔断", exact: true }).click();
    const fuseDialog = page.getByRole("dialog", { name: "手动熔断", exact: true });
    await expect(fuseDialog).toContainText("ID：41");
    expect(actions).toEqual([]);
    await fuseDialog.getByRole("button", { name: "确认手动熔断" }).click();
    await expect(card.getByRole("button", { name: "解除熔断", exact: true })).toBeEnabled();
    await card.getByRole("button", { name: "解除熔断", exact: true }).click();
    await page
      .getByRole("dialog", { name: "解除熔断", exact: true })
      .getByRole("button", { name: "确认解除熔断" })
      .click();
    await expect(card.getByRole("button", { name: "手动熔断", exact: true })).toBeEnabled();
    expect(actions).toEqual(["fuse", "recover"]);
    expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
    expect(
      await card
        .locator("footer")
        .evaluate((element) => element.scrollWidth <= element.clientWidth),
    ).toBe(true);
  });
}
