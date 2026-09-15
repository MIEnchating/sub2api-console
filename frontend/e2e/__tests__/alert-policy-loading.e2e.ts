import { expect, test, type Page, type Route } from "@playwright/test";

import {
  groups,
  notificationStatus,
  policy,
} from "../../src/features/alert-policy/components/__tests__/fixtures";

async function holdPolicy(page: Page, theme: string | null): Promise<Route[]> {
  const requests: Route[] = [];
  await page.addInitScript(
    (value) => localStorage.setItem("sub2api-console-theme", value ?? "light"),
    theme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/alerts/policy") {
      requests.push(route);
      return;
    }
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "告警骨架测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": { items: [] },
      "/api/groups": groups,
      "/api/notifications/status": notificationStatus,
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  return requests;
}

test("告警策略读取前后的三张卡保持分栏和列宽，手机按检测阈值通知顺序排列", async ({
  page,
  colorScheme,
}) => {
  const requests = await holdPolicy(page, colorScheme);
  await page.goto("/alert-policy");
  const loading = page.getByRole("status", { name: "正在读取告警策略", exact: true });
  await expect(loading).toHaveCount(1);
  await expect(loading).toHaveAttribute("aria-busy", "true");
  await expect(page.getByRole("button", { name: "保存策略", exact: true })).toBeDisabled();
  const cards = [
    "alert-detection-skeleton",
    "alert-threshold-skeleton",
    "alert-notification-skeleton",
  ].map((id) => page.getByTestId(id));
  for (const card of cards) await expect(card).toBeVisible();
  const bounds = await Promise.all(cards.map(async (card) => (await card.boundingBox())!));
  if (page.viewportSize()!.width >= 1024) {
    expect(bounds[0].y).toBe(bounds[1].y);
    expect(bounds[1].x).toBeGreaterThan(bounds[0].x);
    expect(bounds[2].x).toBe(bounds[1].x);
    expect(bounds[2].y).toBeGreaterThanOrEqual(bounds[1].y + bounds[1].height);
  } else {
    expect(bounds[1].x).toBe(bounds[0].x);
    expect(bounds[1].y).toBeGreaterThanOrEqual(bounds[0].y + bounds[0].height);
    expect(bounds[2].y).toBeGreaterThanOrEqual(bounds[1].y + bounds[1].height);
  }
  for (const control of await loading.locator('[data-slot="skeleton-control"]').all()) {
    await expect(control).toHaveCSS("height", "32px");
  }
  expect(await loading.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("alert-policy-loading.png") });
  await expect.poll(() => requests.length).toBe(1);
  await requests[0].fulfill({ json: policy });
  await expect(loading).toHaveCount(0);
  const ready = ["告警检测", "阈值与范围", "通知发送"].map((name) =>
    page.getByRole("region", { name, exact: true }),
  );
  for (let index = 0; index < ready.length; index += 1) {
    await expect(ready[index]).toBeVisible();
    expect((await ready[index].boundingBox())!.width).toBe(bounds[index].width);
  }
  await expect(page.getByRole("button", { name: "保存策略", exact: true })).toBeEnabled();
  expect(
    await page
      .locator('[data-slot="page-content"]')
      .evaluate((node) => node.scrollWidth <= node.clientWidth),
  ).toBe(true);
});

test("告警策略刷新失败和重试期间保留表单与草稿，读取成功后恢复保存", async ({
  page,
  colorScheme,
}) => {
  const requests = await holdPolicy(page, colorScheme);
  await page.goto("/alert-policy");
  await expect.poll(() => requests.length).toBe(1);
  await requests[0].fulfill({ json: policy });
  const threshold = page.getByRole("spinbutton", { name: "连续主动探测失败次数", exact: true });
  await expect(threshold).toHaveValue("3");
  await threshold.fill("7");
  const refresh = page.getByRole("button", { name: "刷新告警策略", exact: true });
  const save = page.getByRole("button", { name: "保存策略", exact: true });
  await refresh.click();
  await expect.poll(() => requests.length).toBe(2);
  await expect(save).toBeDisabled();
  await expect(threshold).toHaveValue("7");
  await expect(page.getByRole("status", { name: "正在读取告警策略" })).toHaveCount(0);
  const failure = { status: 503, json: { detail: "隔离测试：策略暂时不可用" } };
  await page.route("**/api/alerts/policy", (route) => route.fulfill(failure));
  await requests[1].fulfill(failure);
  await expect(refresh).toBeEnabled({ timeout: 15_000 });
  await expect(threshold).toHaveValue("7");
  await expect(save).toBeDisabled();
  await page.unroute("**/api/alerts/policy");
  await refresh.click();
  await expect.poll(() => requests.length).toBe(3);
  await requests[2].fulfill({ json: { ...policy, probe_failure_streak: 4 } });
  await expect(save).toBeEnabled();
  await expect(threshold).toHaveValue("7");
});
