import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/automation/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, target_configured: true },
      "/api/auth/session": { authenticated: true, username: "operator" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/overview": { open_alerts: 0 },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/events": [],
      "/api/dictionaries": { items: [] },
    };
    return route.fulfill({ json: fixtures[path] ?? {} });
  });
});

test("键盘切换侧栏时展开状态与界面一致，移动端Escape关闭并恢复焦点", async ({ page }, testInfo) => {
  const mobile = testInfo.project.name === "mobile-dark";
  await page.goto("/profile");
  const trigger = page.getByRole("button", { name: "切换侧边栏" });
  await expect(trigger).toHaveAttribute("aria-expanded", String(!mobile));
  await trigger.focus();
  await page.keyboard.press("Enter");
  // The modal correctly removes the outside trigger from the accessibility tree.
  await expect(page.locator('[data-sidebar="trigger"]')).toHaveAttribute(
    "aria-expanded",
    String(mobile),
  );
  if (mobile) {
    await expect(page.getByRole("dialog", { name: "导航菜单" })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "导航菜单" })).toBeHidden();
    await expect(trigger).toBeFocused();
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
  } else {
    await page.keyboard.press("Control+b");
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
  }
});

test("菜单显隐保存后刷新保持一致，固定设置入口可用且隐藏页面仍能直接访问", async ({
  page,
}, testInfo) => {
  const mobile = testInfo.project.name === "mobile-dark";
  let stored = { hidden_item_ids: [] as string[], version: "v1" };
  const writes: unknown[] = [];
  await page.route("**/api/preferences/navigation", (route) => {
    if (route.request().method() === "PUT") {
      const payload = route.request().postDataJSON();
      writes.push(payload);
      stored = { ...payload, version: "v2" };
    }
    return route.fulfill({ json: stored });
  });
  await page.goto("/config?tab=interface");
  const overview = page.getByRole("switch", { name: "运营总览", exact: true });
  await expect(overview).toHaveAttribute("aria-checked", "true");
  await overview.focus();
  await page.keyboard.press("Space");
  await expect(overview).toHaveAttribute("aria-checked", "false");
  expect(writes).toEqual([{ hidden_item_ids: ["overview"], version: "v1" }]);
  await page.reload();
  await expect(overview).toHaveAttribute("aria-checked", "false");
  await expect(page.getByRole("switch", { name: "系统设置", exact: true })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  if (mobile) await page.getByRole("button", { name: "切换侧边栏" }).click();
  await expect(page.getByRole("link", { name: "运营总览", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "系统设置", exact: true })).toBeVisible();
  if (mobile) await page.keyboard.press("Escape");
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "运营总览", exact: true })).toBeVisible();
  expect(writes).toHaveLength(1);
});

test("选择导航后展示目标页面，移动抽屉关闭且当前入口有可访问选中状态", async ({
  page,
}, testInfo) => {
  const mobile = testInfo.project.name === "mobile-dark";
  await page.goto("/profile");
  const trigger = page.getByRole("button", { name: "切换侧边栏" });
  if (mobile) await trigger.click();
  const link = page.getByRole("link", { name: "运营总览", exact: true });
  await link.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "运营总览", exact: true })).toBeVisible();
  if (mobile) {
    await expect(page.getByRole("dialog", { name: "导航菜单" })).toBeHidden();
    await trigger.click();
  }
  await expect(link).toHaveAttribute("aria-current", "page");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("navigation.png"), fullPage: true });
});
