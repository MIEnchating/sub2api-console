import { expect, test } from "@playwright/test";
import {
  groups,
  notificationStatus,
  policy,
} from "../../src/features/alert-policy/components/__tests__/fixtures";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/alerts/policy": { ...policy, notify_recovery: true },
      "/api/notifications/status": notificationStatus,
      "/api/groups": groups,
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("宽屏双列、窄屏按阅读顺序堆叠，末尾设置可滚动到达且保存按钮保持可见", async ({
  page,
  viewport,
}) => {
  await page.goto("/alert-policy");
  const detection = page.getByRole("region", { name: "告警检测", exact: true });
  const thresholds = page.getByRole("region", { name: "阈值与范围" });
  await expect(detection).toBeVisible();
  await expect(thresholds).toBeVisible();
  const detectionBounds = (await detection.boundingBox())!;
  const thresholdBounds = (await thresholds.boundingBox())!;
  if (viewport!.width >= 1024) {
    expect(thresholdBounds.x).toBeGreaterThanOrEqual(detectionBounds.x + detectionBounds.width);
    expect(thresholdBounds.y).toBe(detectionBounds.y);
  } else {
    expect(thresholdBounds.y).toBeGreaterThanOrEqual(detectionBounds.y + detectionBounds.height);
    expect(thresholdBounds.x).toBe(detectionBounds.x);
  }
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  const lastControl = page.getByRole("switch", { name: "自动执行恢复", exact: true });
  await lastControl.scrollIntoViewIfNeeded();
  await expect(lastControl).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("button", { name: "保存策略", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await content.evaluate((element) => element.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath("alert-policy.png") });
});

test("多个阈值和超长分组名换行后，页面无横向溢出且删除按钮仍可点击", async ({ page }) => {
  const longGroup = "生产环境-".repeat(20);
  await page.route("**/api/groups", (route) =>
    route.fulfill({ json: [{ ...groups[0], name: longGroup }] }),
  );
  await page.route("**/api/alerts/policy", (route) =>
    route.fulfill({ json: { ...policy, probe_groups: [longGroup] } }),
  );
  await page.goto("/alert-policy");
  const add = page.getByRole("button", { name: "添加阈值" });
  await add.click();
  await add.click();
  await expect(page.getByRole("textbox", { name: "余额告警阈值 5", exact: true })).toBeVisible();
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.getByRole("button", { name: "删除余额告警阈值 5", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "余额告警阈值 5", exact: true })).toHaveCount(0);
});

test("大屏暗色主题使用紧凑的开关行", async ({ page }) => {
  await page.setViewportSize({ width: 1920, height: 959 });
  await page.addInitScript(() => localStorage.setItem("sub2api-console-theme", "dark"));
  await page.goto("/alert-policy");
  const row = page
    .locator('[data-slot="setting-switch"]')
    .filter({ has: page.getByRole("switch", { name: "配置异常", exact: true }) });
  await expect(row).toBeVisible();
  const bounds = (await row.boundingBox())!;
  expect(bounds.height).toBeGreaterThanOrEqual(44);
  expect(bounds.height).toBeLessThan(64);
  await expect(page.getByRole("switch", { name: "自动执行恢复", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await expect(page.getByRole("switch", { name: "其他降级原因", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.screenshot({ path: test.info().outputPath("alert-policy-desktop-dark.png") });
});
