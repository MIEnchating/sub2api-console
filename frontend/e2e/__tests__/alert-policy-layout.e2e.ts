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

test("成本流量告警可独立关闭并保存，窄屏无横向溢出", async ({ page }) => {
  let saved = { ...policy };
  await page.route("**/api/alerts/policy", async (route) => {
    if (route.request().method() !== "GET") saved = route.request().postDataJSON();
    await route.fulfill({ json: saved });
  });
  await page.goto("/alert-policy");
  const control = page.getByRole("switch", { name: "无利润／亏损流量", exact: true });
  await expect(control).toBeChecked();
  await control.scrollIntoViewIfNeeded();
  await control.focus();
  await page.keyboard.press("Space");
  await expect(control).not.toBeChecked();
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await expect(page.getByText("告警策略已保存", { exact: true })).toBeVisible();
  expect(saved.cost_traffic_enabled).toBe(false);
  expect(saved.multiplier_increase_enabled).toBe(true);
  await page.reload();
  await expect(control).not.toBeChecked();
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await control.scrollIntoViewIfNeeded();
  await page.screenshot({ path: test.info().outputPath("cost-traffic-policy.png") });
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

test("阈值和发送频率校验失败时，错误完整换行且不覆盖字段或按钮", async ({ page }) => {
  await page.goto("/alert-policy");
  const threshold = page.getByRole("textbox", { name: "余额告警阈值 1", exact: true });
  await expect(threshold).toBeVisible();
  const columns = page.locator('[data-slot="alert-policy-columns"]');
  await threshold.fill("无效阈值");
  await page.getByRole("spinbutton", { name: "重复提醒间隔（分钟）", exact: true }).fill("-1");
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await expect(threshold).toHaveAttribute("aria-invalid", "true");
  const errors = columns.getByRole("alert");
  await expect(errors).toHaveCount(2);
  for (const error of await errors.all()) {
    await expect(error).toBeVisible();
    await expect(error).toHaveCSS("position", "static");
    expect(await error.evaluate((element) => element.scrollHeight <= element.clientHeight)).toBe(
      true,
    );
  }
  const overlaps = await columns.evaluate((element) => {
    const controls = Array.from(element.querySelectorAll("input,button"));
    return Array.from(element.querySelectorAll('[role="alert"]')).some((error) => {
      const bounds = error.getBoundingClientRect();
      return controls.some((control) => {
        const box = control.getBoundingClientRect();
        return (
          box.width > 0 &&
          box.height > 0 &&
          box.left < bounds.right &&
          box.right > bounds.left &&
          box.top < bounds.bottom &&
          box.bottom > bounds.top
        );
      });
    });
  });
  expect(overlaps).toBe(false);
  await threshold.scrollIntoViewIfNeeded();
  await page.screenshot({ path: test.info().outputPath("inline-field-errors.png") });
  await threshold.fill("20");
  await page.getByRole("spinbutton", { name: "重复提醒间隔（分钟）", exact: true }).fill("0");
  await expect(errors).toHaveCount(0);
});
