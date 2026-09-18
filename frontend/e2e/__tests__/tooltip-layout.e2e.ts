import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test, type Locator, type Page } from "@playwright/test";

let bundleDirectory: string;
test.beforeAll(() => {
  bundleDirectory = mkdtempSync(join(tmpdir(), "console-tooltip-layout-"));
  execFileSync(
    "bun",
    ["build", "e2e/__tests__/fixtures/tooltip-layout.tsx", "--outdir", bundleDirectory],
    { cwd: process.cwd(), stdio: "pipe" },
  );
});
test.afterAll(() => rmSync(bundleDirectory, { recursive: true, force: true }));

async function openTooltipFixture(
  page: Page,
  surface: string,
  theme: string | null,
): Promise<void> {
  page.on("pageerror", (error) => {
    throw error;
  });
  await page.route("**/api/**", () => {});
  await page.goto("/");
  await expect(page.getByRole("status")).toBeVisible();
  const css = await page.evaluate(() =>
    Array.from(document.styleSheets)
      .flatMap((sheet) => Array.from(sheet.cssRules).map((rule) => rule.cssText))
      .join("\n"),
  );
  await page.route("**/tooltip-layout.html?*", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: '<html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><main id="tooltip-root" style="padding:120px 16px;max-width:480px;margin:0 auto"></main></body></html>',
    }),
  );
  await page.goto(`/tooltip-layout.html?surface=${surface}`);
  await page.addStyleTag({ content: css });
  await page.evaluate(
    (value) => document.documentElement.classList.toggle("dark", value === "dark"),
    theme,
  );
  await page.addScriptTag({ path: join(bundleDirectory, "tooltip-layout.js"), type: "module" });
}

async function expectTooltipToContainText(tooltip: Locator): Promise<void> {
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toBeInViewport({ ratio: 1 });
  await expect
    .poll(() =>
      tooltip.evaluate((element) => {
        const popup = element.getBoundingClientRect();
        const style = getComputedStyle(element);
        const content = {
          left: popup.left + parseFloat(style.borderLeftWidth) + parseFloat(style.paddingLeft),
          right: popup.right - parseFloat(style.borderRightWidth) - parseFloat(style.paddingRight),
          top: popup.top + parseFloat(style.borderTopWidth) + parseFloat(style.paddingTop),
          bottom:
            popup.bottom - parseFloat(style.borderBottomWidth) - parseFloat(style.paddingBottom),
        };
        const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
        let node = walker.nextNode();
        let overflow = 0;
        while (node) {
          if (node.textContent?.trim()) {
            const range = document.createRange();
            range.selectNodeContents(node);
            for (const bounds of range.getClientRects()) {
              overflow = Math.max(
                overflow,
                content.left - bounds.left,
                bounds.right - content.right,
                content.top - bounds.top,
                bounds.bottom - content.bottom,
              );
            }
          }
          node = walker.nextNode();
        }
        return overflow;
      }),
    )
    .toBeLessThanOrEqual(1);
}

test("账号探针长摘要与错误详情完整包裹，320px 窄屏自动换行", async ({ page, colorScheme }) => {
  const viewport = page.viewportSize()!;
  for (const surface of ["probe", "probe-failure"]) {
    await page.setViewportSize(viewport);
    await openTooltipFixture(page, surface, colorScheme);
    const trigger = page.getByLabel(/首字 3303ms · 探针/);
    await trigger.hover();
    const tooltip = page.locator('[data-slot="tooltip-content"]');
    await expect(tooltip).toContainText("首字 3303ms · 探针");
    if (surface === "probe-failure") await expect(tooltip).toContainText("上游连接失败");
    await expectTooltipToContainText(tooltip);
    if (viewport.width > 640) {
      expect((await tooltip.boundingBox())!.width).toBeGreaterThan(320);
    }
    await page.screenshot({ path: test.info().outputPath(`${surface}-tooltip.png`) });
    await page.setViewportSize({ width: 320, height: 480 });
    await trigger.hover();
    await expectTooltipToContainText(tooltip);
    expect((await tooltip.boundingBox())!.height).toBeGreaterThan(50);
    await page.screenshot({ path: test.info().outputPath(`${surface}-tooltip-320px.png`) });
  }
});

test("默认提示中的无空格模型名和 URL 在窄屏完整换行", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 320, height: 480 });
  for (const surface of ["name", "url"]) {
    await openTooltipFixture(page, surface, colorScheme);
    await page.getByRole("button", { name: "查看说明" }).hover();
    await expectTooltipToContainText(page.locator('[data-slot="tooltip-content"]'));
  }
});

test("自定义最大宽度允许长提示超过默认宽度，短提示按内容收缩", async ({ page, colorScheme }) => {
  await openTooltipFixture(page, "custom", colorScheme);
  await page.getByRole("button", { name: "查看说明" }).hover();
  const tooltip = page.locator('[data-slot="tooltip-content"]');
  await expectTooltipToContainText(tooltip);
  if (page.viewportSize()!.width > 640) {
    expect((await tooltip.boundingBox())!.width).toBeGreaterThan(320);
  }
  await openTooltipFixture(page, "short", colorScheme);
  await page.getByRole("button", { name: "查看说明" }).hover();
  await expectTooltipToContainText(tooltip);
  expect((await tooltip.boundingBox())!.width).toBeLessThan(200);
});

test("键盘聚焦账号结果显示完整提示，Escape 关闭并保留焦点", async ({ page, colorScheme }) => {
  await openTooltipFixture(page, "probe", colorScheme);
  const trigger = page.getByLabel(/探测通过 · 100 分 · 首字 3303ms · 探针/);
  await expect(trigger).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(trigger).toBeFocused();
  const tooltip = page.locator('[data-slot="tooltip-content"]');
  await expectTooltipToContainText(tooltip);
  await page.keyboard.press("Escape");
  await expect(tooltip).toBeHidden();
  await expect(trigger).toBeFocused();
});

for (const side of ["top", "bottom", "left", "right", "flip"]) {
  test(`共享提示位于 ${side} 时鼠标可跨越间隙进入并选择文字`, async ({ page, colorScheme }) => {
    await page.setViewportSize({ width: 800, height: 600 });
    await openTooltipFixture(page, `hover-${side}`, colorScheme);
    const trigger = page.getByRole("button", { name: "查看地址" });
    await trigger.hover();
    const tooltip = page.getByRole("tooltip");
    await expect(tooltip).toHaveText("app.example.test");
    const actualSide = side === "flip" ? "bottom" : side;
    await expect(tooltip).toHaveAttribute("data-side", actualSide);
    const anchor = (await trigger.boundingBox())!;
    const popup = (await tooltip.boundingBox())!;
    const point = { x: anchor.x + anchor.width / 2, y: anchor.y + anchor.height / 2 };
    if (actualSide === "top") point.y = (anchor.y + popup.y + popup.height) / 2;
    if (actualSide === "bottom") point.y = (anchor.y + anchor.height + popup.y) / 2;
    if (actualSide === "left") point.x = (anchor.x + popup.x + popup.width) / 2;
    if (actualSide === "right") point.x = (anchor.x + anchor.width + popup.x) / 2;
    await page.mouse.move(point.x, point.y, { steps: 8 });
    expect(
      await tooltip.evaluate(
        (element, coordinates) =>
          element.contains(document.elementFromPoint(coordinates.x, coordinates.y)),
        point,
      ),
    ).toBe(true);
    await tooltip.hover();
    await expect(tooltip).toHaveText("app.example.test");
    await tooltip.click({ clickCount: 3 });
    expect(await page.evaluate(() => window.getSelection()?.toString())).toBe("app.example.test");
    await page.mouse.move(0, 599, { steps: 8 });
    await expect(tooltip).toBeHidden();
  });
}
