import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page }) => {
  const accounts = Array.from({ length: 25 }, (_, index) => ({
    ...account,
    id: String(index + 1),
    name: `滚动测试账号 ${index + 1}`,
    platform: "openai",
  }));
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/accounts": accounts,
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ status: 200, contentType: "text/event-stream", body: "" });
      return;
    }
    if (/\/accounts\/\d+\/models$/.test(path)) {
      await route.fulfill({ json: { models: ["test-model"] } });
      return;
    }
    await route.fulfill({ json: fixtures[path] ?? [] });
  });
});

for (const linkedAccount of [false, true]) {
  test(`${linkedAccount ? "账号入口切换全部账号" : "全部账号入口"}在长列表下保持表格内部滚动和设置可达`, async ({
    page,
  }) => {
    await page.goto(linkedAccount ? "/model-check?account_id=1" : "/model-check");
    const submit = page.getByRole("button", { name: /开始检测/ });
    const desktop = (page.viewportSize()?.width ?? 0) >= 1280;
    if (linkedAccount) {
      if (desktop) await expect(submit).toBeInViewport({ ratio: 1 });
      await page.getByRole("button", { name: "查看全部账号" }).click();
    }
    const table = page.getByTestId("model-check-account-desktop-table");
    const list = desktop ? table : page.getByTestId("model-check-account-mobile-list");
    await expect(list.getByText("滚动测试账号 20", { exact: true })).toBeAttached();
    const scrollContainer = list.locator("..");
    await expect
      .poll(() =>
        scrollContainer.evaluate((element) => element.scrollHeight > element.clientHeight),
      )
      .toBe(true);

    if (desktop) {
      const pageContent = page.locator('[data-slot="page-content"]');
      await expect
        .poll(() => pageContent.evaluate((element) => element.scrollHeight - element.clientHeight))
        .toBeLessThanOrEqual(1);
      await expect(submit).toBeInViewport({ ratio: 1 });
      const filter = page.getByRole("group", { name: "账号搜索与选择" });
      const filterBefore = await filter.boundingBox();
      const submitBefore = await submit.boundingBox();
      await scrollContainer.hover();
      await page.mouse.wheel(0, 1200);
      await expect
        .poll(() => scrollContainer.evaluate((element) => element.scrollTop))
        .toBeGreaterThan(0);
      expect(await filter.boundingBox()).toEqual(filterBefore);
      expect(await submit.boundingBox()).toEqual(submitBefore);
      await expect(page.getByRole("button", { name: "转到第 2 页" })).toBeInViewport({ ratio: 1 });
    } else {
      await list.getByText("滚动测试账号 20", { exact: true }).scrollIntoViewIfNeeded();
      await expect
        .poll(() => scrollContainer.evaluate((element) => element.scrollTop))
        .toBeGreaterThan(0);
      await submit.scrollIntoViewIfNeeded();
      await expect(submit).toBeInViewport({ ratio: 1 });
    }
  });
}
