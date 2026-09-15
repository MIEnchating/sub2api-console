import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const model = {
    model: "claude-fable-5",
    input_ratio: "5",
    completion_ratio: "5",
    billing_mode: "tiered_expr",
    billing_expr: 'tier("base", p * 10 + c * 50 + cr * 1 + cc * 12.5 + cc1h * 20)',
  };
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "弹窗滚动复查" },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/refresh": {
        groups: [],
        models: [model],
        unset_models: [],
        references: [],
        tool_prices: [],
        differences: [],
        upstream_prices: [
          {
            host: "upstream.example.test",
            name: "比对上游",
            upstream_type: "sub2api",
            models: [{ model: model.model, input_ratio: "5", completion_ratio: "5" }],
          },
        ],
      },
    };
    if (path.endsWith("/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    if (path in fixtures) return route.fulfill({ json: fixtures[path] });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

for (const height of [480, 900]) {
  test(`${height}px 高度下价格比对正文独立滚动，标题和关闭按钮留在原位`, async ({
    page,
    viewport,
  }) => {
    await page.setViewportSize({ width: viewport!.width, height });
    await page.goto("/newapi/differences");
    await page.getByRole("combobox", { name: "比对上游" }).click();
    await page.getByRole("option", { name: "比对上游 · Sub2API", exact: true }).click();
    await page.getByRole("button", { name: "比对 claude-fable-5", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "claude-fable-5 价格比对" });
    const title = dialog.getByRole("heading", { name: "claude-fable-5 价格比对" });
    const close = dialog.getByRole("button", { name: "关闭", exact: true });
    const body = dialog.locator('[data-slot="dialog-body"]');
    await close.click({ trial: true });
    await expect(title).toBeInViewport({ ratio: 1 });
    await expect
      .poll(() => dialog.evaluate((element) => element.scrollHeight <= element.clientHeight))
      .toBe(true);
    const initialTitle = await title.boundingBox();
    const initialClose = await close.boundingBox();
    const overflowing = await body.evaluate(
      (element) => element.scrollHeight > element.clientHeight,
    );
    if (height === 480) expect(overflowing).toBe(true);
    await body.evaluate((element) => element.scrollTo(0, element.scrollHeight));
    if (overflowing)
      await expect.poll(() => body.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await expect(dialog.locator("tbody tr").last().locator("td").first()).toBeInViewport({
      ratio: 1,
    });
    await expect(title).toBeInViewport({ ratio: 1 });
    expect(await title.boundingBox()).toEqual(initialTitle);
    expect(await close.boundingBox()).toEqual(initialClose);
    await close.click({ trial: true });
    await page.screenshot({ path: test.info().outputPath("comparison-scrolled.png") });
    const tableContainer = dialog.locator('[data-slot="table-container"]');
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    if (viewport!.width < 600) {
      await tableContainer.evaluate((element) => element.scrollTo(element.scrollWidth, 0));
      await expect
        .poll(() => tableContainer.evaluate((element) => element.scrollLeft))
        .toBeGreaterThan(0);
      await expect(
        dialog.locator("tbody tr").last().getByText("一致", { exact: true }),
      ).toBeInViewport({
        ratio: 1,
      });
      await expect(title).toBeInViewport({ ratio: 1 });
      await page.screenshot({ path: test.info().outputPath("comparison-right-columns.png") });
    }
    await close.click();
    await expect(dialog).toHaveCount(0);
  });
}
