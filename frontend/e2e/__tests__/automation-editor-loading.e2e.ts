import { expect, test } from "@playwright/test";
import { pageFixtures, workspace } from "./fixtures/page-shell";

test("渠道按需读取，加载期间可关闭，重试后焦点进入表单且没有嵌套弹窗", async ({ page }) => {
  let channelReads = 0;
  let phase: "loading" | "error" | "ready" = "loading";
  let release!: () => void;
  const waiting = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/channels")) {
      channelReads += 1;
      const requestPhase = phase;
      if (requestPhase === "loading") await waiting;
      if (requestPhase === "error") {
        await route.fulfill({ status: 503, json: { detail: "隔离测试：渠道暂不可用" } });
      } else await route.fulfill({ json: { items: [], total: 0 } });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离测试" },
      "/api/newapi": workspace,
      "/api/groups": [],
      "/api/management/accounts/models/automation/schedules": [],
      "/api/management/accounts/models/automation/task-groups": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else await route.fulfill({ json: fixtures[path] ?? [] });
  });
  try {
    await page.goto("/newapi/account-model-automation");
    await expect(
      page.getByText("暂无运营任务，点击右上角“添加任务”创建。", { exact: true }),
    ).toBeVisible();
    expect(channelReads).toBe(0);
    await page.getByRole("button", { name: "添加任务", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "添加运营任务" });
    await expect(dialog.getByRole("status", { name: "正在读取渠道" })).toBeVisible();
    await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeEnabled();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    phase = "error";
    release();
    await page.getByRole("button", { name: "添加任务", exact: true }).click();
    const retry = dialog.getByRole("button", { name: "重新读取", exact: true });
    await expect(retry).toBeVisible();
    phase = "ready";
    await retry.click();
    const name = dialog.getByRole("textbox", { name: "运营任务名称" });
    await expect(name).toBeVisible();
    await expect(name).toBeFocused();
    await expect(page.getByRole("dialog")).toHaveCount(1);
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await page.screenshot({ path: test.info().outputPath("editor-loaded.png") });
  } finally {
    release();
  }
});
