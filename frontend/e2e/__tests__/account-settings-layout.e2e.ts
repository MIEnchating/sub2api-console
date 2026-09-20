import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

const accountName = "mdkj-0.07 / " + "长账号名称用于检查换行".repeat(8);

test("账号设置在桌面分栏、小屏单栏，滚动时保留操作且独立开关保存后生效", async ({
  page,
}, testInfo) => {
  let allocationEnabled = false;
  let settingsWrites = 0;
  let allocationWrites = 0;
  const row = {
    ...account,
    id: "41",
    name: accountName,
    upstream_type: "sub2api",
    priority: 7661,
    concurrency: 6,
    load_factor: "1",
  };
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme),
    testInfo.project.name.includes("dark") ? "dark" : "light",
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/policy/upstream-concurrency/accounts/41") {
      if (route.request().method() === "PUT") {
        allocationWrites++;
        allocationEnabled = route.request().postDataJSON().override === true;
      }
      await route.fulfill({
        json: {
          revision: "v1",
          target_id: "41",
          upstream_id: "up_test",
          override: allocationEnabled ? true : null,
          selected: allocationEnabled,
          effective: allocationEnabled,
          global_enabled: true,
          source: allocationEnabled ? "account" : "policy",
        },
      });
      return;
    }
    if (path === "/api/accounts/41/settings") settingsWrites++;
    const responses: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离布局测试" },
      "/api/accounts": [row],
      "/api/accounts/41": {
        ...row,
        metadata: {},
        group_rates: {},
        group_ids: {},
        bindings: [],
        test_models: ["gpt-5.1-codex"],
      },
      "/api/model-checks/account-statuses": [],
      "/api/groups": [],
      "/api/policy": { available: true, advanced_policy: {} },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in responses && route.request().method() === "GET")
      await route.fulfill({ json: responses[path] });
    else await route.fulfill({ status: 503, json: { detail: "未配置的隔离接口" } });
  });
  await page.goto("/accounts");
  await page.getByRole("button", { name: "更多账号操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "查看并编辑账号", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "账号设置", exact: true });
  const routing = dialog.getByRole("region", { name: "调度参数", exact: true });
  const control = dialog.getByRole("region", { name: "账号管控", exact: true });
  const independent = dialog.getByRole("region", { name: "独立策略", exact: true });
  await expect(independent).toContainText("修改后点击保存生效");
  const shared = independent.getByRole("switch", { name: "上游共享并发分配", exact: true });
  await expect(shared).not.toBeChecked();
  await dialog.evaluate(async (element) => {
    await Promise.all(element.getAnimations().map((animation) => animation.finished));
  });
  const bounds = await dialog.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width - 30);
  const routingBox = (await routing.boundingBox())!;
  const controlBox = (await control.boundingBox())!;
  if (page.viewportSize()!.width >= 768) {
    expect(controlBox.x).toBeGreaterThan(routingBox.x + routingBox.width);
    expect(Math.abs(controlBox.y - routingBox.y)).toBeLessThan(2);
    expect(bounds!.height).toBeLessThan(650);
  } else {
    expect(controlBox.y).toBeGreaterThan(routingBox.y + routingBox.height);
  }
  const save = dialog.getByRole("button", { name: "保存", exact: true });
  const cancel = dialog.getByRole("button", { name: "取消", exact: true });
  const initialSave = (await save.boundingBox())!;
  expect(initialSave.height).toBe(32);
  expect((await cancel.boundingBox())!.y).toBe(initialSave.y);
  await shared.focus();
  await page.keyboard.press("Space");
  await expect(shared).toBeChecked();
  await expect(independent.getByRole("button", { name: "恢复跟随" })).toHaveCount(0);
  await expect(independent.getByText("未纳入分配范围")).toHaveCount(0);
  await expect(independent.getByText("跟随调度策略范围")).toHaveCount(0);
  expect(allocationWrites).toBe(0);
  expect(settingsWrites).toBe(0);
  expect(Math.abs((await save.boundingBox())!.y - initialSave.y)).toBeLessThan(2);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const body = dialog.locator('[data-slot="dialog-body"]');
  if (page.viewportSize()!.width >= 768) {
    const resetBox = (await shared.boundingBox())!;
    const bodyBox = (await body.boundingBox())!;
    expect(resetBox.y + resetBox.height).toBeLessThanOrEqual(bodyBox.y + bodyBox.height);
  }
  expect(await body.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("account-settings.png") });
  await cancel.click();
  await expect(dialog).toHaveCount(0);
  expect(allocationWrites).toBe(0);
  expect(settingsWrites).toBe(0);
});
