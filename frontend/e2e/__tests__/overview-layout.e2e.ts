import { expect, test } from "@playwright/test";
import {
  overviewAccount,
  overviewGroup,
  overviewEvent,
  groupName,
  attentionReason,
} from "./fixtures/overview";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "总览布局测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/accounts": [overviewAccount],
      "/api/groups": [overviewGroup],
      "/api/events": [overviewEvent],
    };
    if (path.endsWith("/automation/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures && route.request().method() === "GET")
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

test("长分组名、风险原因和事件摘要完整换行，滚动后导航操作仍可用", async ({ page }) => {
  await page.goto("/");
  const title = page.getByRole("heading", { name: groupName, exact: true });
  await expect(title).toBeVisible();
  expect(await title.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  for (const value of ["仅剩保底", attentionReason, overviewEvent.summary]) {
    const text = page.getByText(value, { exact: true });
    await text.scrollIntoViewIfNeeded();
    expect(await text.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
    expect(await text.evaluate((element) => element.scrollHeight <= element.clientHeight)).toBe(
      true,
    );
  }
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await expect(page.getByRole("heading", { name: "运营总览", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await expect(page.getByRole("button", { name: "立即同步", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await content.evaluate((element) => element.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath("overview.png") });
  await page.getByRole("button", { name: "打开分组管理" }).click();
  await expect(page).toHaveURL(/\/groups$/);
});

test("首次读取失败时显示明确错误和缺失指标，刷新成功后恢复健康矩阵", async ({ page }) => {
  let failed = true;
  await page.route("**/api/groups", (route) =>
    failed
      ? route.fulfill({ status: 503, json: { detail: "分组数据暂时不可用" } })
      : route.fulfill({ json: [overviewGroup] }),
  );
  await page.goto("/");
  const matrix = page.getByTestId("group-health-grid");
  const errorMessage = page
    .locator("[data-sonner-toast]")
    .filter({ hasText: "分组数据暂时不可用" });
  await expect(errorMessage).toHaveCount(1, { timeout: 15000 });
  await expect(matrix).not.toContainText("分组数据暂时不可用");
  await expect(
    page.getByRole("region", { name: "核心运营指标" }).getByText("—", { exact: true }),
  ).toHaveCount(4);
  failed = false;
  await page.getByRole("button", { name: "刷新运营总览" }).click();
  await expect(page.getByRole("heading", { name: groupName, exact: true })).toBeVisible();
});
