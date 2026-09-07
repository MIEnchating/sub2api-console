import { expect, test } from "@playwright/test";

for (const modelCount of [2, 16]) {
  test(`异步加载 ${modelCount} 行弹窗表格时不产生临时滚动条，保留实际溢出滚动`, async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "no-preference" });
    const models = Array.from({ length: modelCount }, (_, index) => ({
      model: `model-${String(index).padStart(2, "0")}`,
      input_ratio: "1",
      completion_ratio: "4",
    }));
    let releasePrices = (): void => {};
    const pricesReady = new Promise<void>((resolve) => {
      releasePrices = resolve;
    });
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path.endsWith("/management-model-prices")) {
        await pricesReady;
        await route.fulfill({
          json: {
            models: models.map((model) => ({
              model: model.model,
              source: "sub2api",
              input_price: "0.000001",
              output_price: "0.000004",
              model_ratio: "0.5",
              completion_ratio: "4",
            })),
          },
        });
        return;
      }
      const fixtures: Record<string, unknown> = {
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "弹窗测试" },
        "/api/inspection/automation": {
          enabled: false,
          running: false,
          traffic_collection: { enabled: false },
        },
        "/api/newapi": {
          platforms: [
            {
              id: "test-platform",
              name: "测试平台",
              base_url: "https://newapi.example",
              user_id: "1",
              admin_key_configured: true,
            },
          ],
          local_groups: [],
          bindings: [],
        },
        "/api/newapi/platforms/test-platform/refresh": {
          groups: [],
          models,
          unset_models: [],
          references: [],
          tool_prices: [],
          differences: [],
        },
      };
      if (path.endsWith("/events")) {
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
      } else if (path in fixtures) {
        await route.fulfill({ json: fixtures[path] });
      } else {
        await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
      }
    });

    await page.goto("/newapi/prices");
    await page.evaluate(() => document.fonts.ready);
    await page.getByRole("checkbox", { name: "选择本页模型", exact: true }).check();
    await page.getByRole("button", { name: `批量同步（${modelCount}）` }).click();
    const dialog = page.getByRole("dialog", { name: "批量同步模型价格" });
    await expect(dialog.getByRole("status")).toHaveText("正在准备批量价格预览…");
    releasePrices();
    await expect(
      dialog.getByRole("button", { name: `确认同步 ${modelCount} 个模型` }),
    ).toBeEnabled();
    await expect(dialog.locator("tbody tr")).toHaveCount(modelCount);

    // Sample the actual CSS animation at deterministic points rather than
    // sleeping until the transient overflow has already disappeared.
    const samples = await dialog.evaluate((element) => {
      const animations = Array.from(element.querySelectorAll("tbody tr")).flatMap((row) =>
        row.getAnimations(),
      );
      const regions = [
        element,
        ...element.querySelectorAll('[data-slot="dialog-body"], [data-slot="table-container"]'),
      ];
      const measure = () =>
        regions.map((region) => ({
          slot: region.getAttribute("data-slot"),
          verticalOverflow: Math.max(0, region.scrollHeight - region.clientHeight),
          horizontalOverflow: Math.max(0, region.scrollWidth - region.clientWidth),
        }));
      for (const animation of animations) {
        animation.pause();
        animation.currentTime = 0;
      }
      const entering = measure();
      for (const animation of animations) animation.finish();
      return { entering, settled: measure() };
    });
    expect(samples.entering).toEqual(samples.settled);

    const tableContainer = dialog.locator('[data-slot="table-container"]');
    if (modelCount === 2) {
      expect(samples.settled.every((region) => region.verticalOverflow === 0)).toBe(true);
    } else {
      expect(
        samples.settled.find((region) => region.slot === "table-container")?.verticalOverflow,
      ).toBeGreaterThan(0);
      await tableContainer.evaluate((element) => {
        element.scrollTop = element.scrollHeight;
      });
      await expect(
        dialog.getByText(models[modelCount - 1]!.model, { exact: true }),
      ).toBeInViewport();
    }
    if (test.info().project.name === "mobile-dark") {
      expect(
        samples.settled.find((region) => region.slot === "table-container")?.horizontalOverflow,
      ).toBeGreaterThan(0);
      await tableContainer.evaluate((element) => {
        element.scrollLeft = element.scrollWidth;
      });
      expect(await tableContainer.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
    }
    await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeInViewport();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });
}
