import { expect, test } from "@playwright/test";
import type { AnimationSchedule } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("批量每天定时保留当前检测类型，影响账号可滚动且窄屏按钮可达", async ({ page }) => {
  const accounts = Array.from({ length: 25 }, (_, index) => ({
    ...account,
    id: String(index + 1),
    name: `定时账号 ${index + 1} ${"较长名称".repeat(8)}`,
    platform: "openai",
  }));
  let schedules: AnimationSchedule[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/animation-schedules" && route.request().method() === "PUT") {
      schedules = route.request().postDataJSON().schedules;
      await route.fulfill({ json: schedules });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "定时测试" },
      "/api/accounts": accounts,
      "/api/model-checks/animation-schedules": schedules,
      "/api/model-checks/animations": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": test\n\n" });
    else if (path.startsWith("/api/dictionaries")) await route.fulfill({ json: { items: [] } });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("tab", { name: "前置检测", exact: true }).click();
  await expect(page.getByRole("button", { name: "批量自动检测设置" })).toBeDisabled();
  await page.getByRole("button", { name: "全选账号" }).click();
  await page.getByRole("button", { name: "批量自动检测设置" }).click();
  const dialog = page.getByRole("dialog", { name: /批量自动前置检测设置/ });
  await dialog.getByRole("checkbox", { name: "开启自动检测" }).check();
  await dialog.getByRole("textbox", { name: "检测模型" }).fill("test-model");
  await dialog.getByRole("radio", { name: "每天定时" }).check();
  await dialog.getByLabel("每天检测时间（北京时间）").fill("23:45");
  await dialog.getByRole("button", { name: "添加检测时间" }).click();
  await dialog.getByLabel("每天检测时间 2（北京时间）").fill("09:00");
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "保存设置" })).toBeInViewport();
  await dialog.getByRole("button", { name: "保存设置" }).click();
  const confirm = page.getByRole("dialog", { name: "确认开启自动检测" });
  await expect(confirm).toContainText("每天 09:00、23:45（北京时间）");
  const list = confirm.getByRole("list", { name: "自动检测影响账号" });
  await expect(list.getByRole("listitem")).toHaveCount(25);
  await expect(list).toHaveCSS("overflow-y", "auto");
  expect(await confirm.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await expect(confirm.getByRole("button", { name: "确认保存并开启" })).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("daily-batch-schedule.png") });
  await confirm.getByRole("button", { name: "确认保存并开启" }).click();
  await expect.poll(() => schedules.length).toBe(25);
  expect(
    schedules.every(
      (item) =>
        item.mode === "precheck" &&
        JSON.stringify(item.daily_times) === '["09:00","23:45"]' &&
        item.timezone === "Asia/Shanghai",
    ),
  ).toBe(true);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByText("每天 09:00、23:45（北京时间）自动检测").first()).toBeVisible();
  await page.getByRole("button", { name: "自动检测设置", exact: true }).first().click();
  const saved = page.getByRole("dialog", { name: /自动前置检测设置/ });
  await expect(saved.getByLabel("每天检测时间（北京时间）")).toHaveValue("09:00");
  await expect(saved.getByLabel("每天检测时间 2（北京时间）")).toHaveValue("23:45");
  await saved.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("tab", { name: "账号检测", exact: true }).click();
  await expect(page.getByText("每天 09:00、23:45（北京时间）自动检测")).toHaveCount(0);
});
