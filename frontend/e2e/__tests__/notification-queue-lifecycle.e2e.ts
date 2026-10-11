import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";
import type { NotificationQueueDetails } from "../../src/api";

const empty: NotificationQueueDetails = {
  producer_firing: [],
  producer_recovered: [],
  consumer_pending: [],
  consumer_failed: [],
  consumer_items: [],
};

for (const scenario of ["cancel", "retry"] as const) {
  test(`告警队列${scenario === "cancel" ? "关闭取消并重开读取" : "失败后键盘重试"}`, async ({
    page,
  }, testInfo) => {
    if (testInfo.project.name === "mobile-dark")
      await page.setViewportSize({ width: 320, height: 568 });
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme),
      testInfo.project.use.colorScheme ?? "light",
    );
    let queueRequests = 0;
    let releaseFirst: () => void = () => undefined;
    const firstReleased = new Promise<void>((resolve) => {
      releaseFirst = resolve;
    });
    await page.route("**/api/**", async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname === "/api/notifications/queue") {
        queueRequests++;
        if (queueRequests === 1) {
          if (scenario === "cancel") await firstReleased;
          else {
            await route.fulfill({ status: 503, json: { detail: "隔离测试：队列暂不可用" } });
            return;
          }
        }
        await route.fulfill({ json: empty });
        return;
      }
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "告警队列测试" },
        "/api/dictionaries": { items: [] },
        "/api/overview": { open_alerts: 0, accounts: 0, groups: 0, upstreams: 0 },
      };
      if (url.pathname.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
      else if (url.pathname in fixtures) await route.fulfill({ json: fixtures[url.pathname] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto("/alerts");
    const opener = page.getByRole("button", { name: "查看队列", exact: true });
    await opener.click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    if (scenario === "cancel") {
      await expect(dialog.getByRole("status", { name: "正在读取队列内容" })).toBeVisible();
      await expect.poll(() => queueRequests).toBe(1);
      const cancelled = page.waitForEvent("requestfailed", {
        predicate: (request) => new URL(request.url()).pathname === "/api/notifications/queue",
      });
      await page.keyboard.press("Escape");
      const failedRequest = await cancelled;
      expect(failedRequest.failure()?.errorText).toContain("ERR_ABORTED");
      await expect(dialog).not.toBeVisible();
      await expect(opener).toBeFocused();
      releaseFirst();
      await opener.press("Enter");
    } else {
      await expect(page.getByText("隔离测试：队列暂不可用", { exact: true })).toBeVisible();
      await expect(dialog.getByText("隔离测试：队列暂不可用", { exact: true })).toHaveCount(0);
      const retry = dialog.getByRole("button", { name: "刷新队列内容" });
      await retry.focus();
      await retry.press("Enter");
    }
    await expect(dialog.getByText("没有匹配的队列内容", { exact: true })).toBeVisible();
    expect(queueRequests).toBe(2);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({ path: testInfo.outputPath(`${scenario}-ready.png`) });
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await expect(opener).toBeFocused();
    releaseFirst();
  });
}
