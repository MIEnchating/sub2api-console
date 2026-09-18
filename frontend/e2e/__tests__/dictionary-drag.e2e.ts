import { expect, test, type Page } from "@playwright/test";
import { waitForLayoutAnimations } from "../layout-motion";
import { pageFixtures } from "./fixtures/page-shell";

async function setupDictionary(page: Page): Promise<string[][]> {
  const writes: string[][] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/dictionaries") {
      await route.fulfill({
        json: {
          items: ["openai", "gemini", "claude"].map((value, i) => ({
            id: value,
            value,
            name: value,
            kind: "platform",
            enabled: true,
            sort_order: i,
            description: "",
            version: 1,
          })),
        },
      });
    } else if (path === "/api/dictionaries/reorder") {
      writes.push(route.request().postDataJSON().ids);
      await route.fulfill({ status: 204 });
    } else if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else {
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "字典测试" },
      };
      await route.fulfill({ json: fixtures[path] ?? {} });
    }
  });
  await page.goto("/config?tab=dictionaries");
  await page.getByRole("tab", { name: "字典管理", exact: true }).click();
  await expect(page.getByRole("button", { name: "拖动openai", exact: true })).toBeVisible();
  await waitForLayoutAnimations(page.getByRole("table"));
  return writes;
}

test("拖拽过程有位移动画，放下只保存一次，搜索中禁用排序", async ({ page }, testInfo) => {
  // Keep this animation assertion away from the vertical autoscroll boundary.
  await page.setViewportSize({ width: page.viewportSize()!.width, height: 900 });
  const writes = await setupDictionary(page);
  const source = page.getByRole("button", { name: "拖动openai", exact: true });
  const target = page.getByRole("button", { name: "拖动gemini", exact: true });
  const from = (await source.boundingBox())!;
  const to = (await target.boundingBox())!;
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 8 });
  await expect(source).toHaveAttribute("aria-pressed", "true");
  const targetRow = page.getByRole("row").filter({ has: target });
  await expect
    .poll(() => targetRow.evaluate((node) => getComputedStyle(node).transform))
    .not.toBe("none");
  await page.screenshot({ path: testInfo.outputPath("dictionary-dragging.png") });
  expect(writes).toEqual([]);
  await page.mouse.up();
  await expect(page.getByRole("row").nth(1)).toContainText("gemini");
  await expect.poll(() => writes).toEqual([["gemini", "openai", "claude"]]);
  await page.getByPlaceholder("搜索名称、字典值或说明").fill("openai");
  await expect(source).toBeDisabled();
});

test("移出列表和 Escape 取消不提交，键盘排序按稳定 ID 提交", async ({ page }) => {
  const writes = await setupDictionary(page);
  const source = page.getByRole("button", { name: "拖动openai", exact: true });
  const box = (await source.boundingBox())!;
  await page.mouse.move(box.x + 16, box.y + 16);
  await page.mouse.down();
  await page.mouse.move(2, 2, { steps: 8 });
  await page.mouse.up();
  await expect(source).toHaveAttribute("aria-pressed", "false");
  expect(writes).toEqual([]);
  await waitForLayoutAnimations(page.getByRole("table"));
  await source.focus();
  await page.keyboard.press("Space");
  await expect(source).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("ArrowDown");
  await expect(page.getByText("目标位置 2", { exact: true })).toBeAttached();
  await page.keyboard.press("Escape");
  await expect(source).toHaveAttribute("aria-pressed", "false");
  expect(writes).toEqual([]);
  await waitForLayoutAnimations(page.getByRole("table"));
  await source.focus();
  await page.keyboard.press("Space");
  await expect(source).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("ArrowDown");
  await expect(page.getByText("目标位置 2", { exact: true })).toBeAttached();
  await page.keyboard.press("Space");
  await expect.poll(() => writes).toEqual([["gemini", "openai", "claude"]]);
});

test("减少动态效果时仍可用触摸拖动完成排序", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const writes = await setupDictionary(page);
  const source = page.getByRole("button", { name: "拖动openai", exact: true });
  const target = page.getByRole("button", { name: "拖动gemini", exact: true });
  const from = (await source.boundingBox())!;
  const to = (await target.boundingBox())!;
  const session = await page.context().newCDPSession(page);
  await session.send("Emulation.setTouchEmulationEnabled", { enabled: true });
  await session.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: from.x + 16, y: from.y + 16 }],
  });
  await session.send("Input.dispatchTouchEvent", {
    type: "touchMove",
    touchPoints: [{ x: from.x + 16, y: from.y + 24 }],
  });
  await expect(source).toHaveAttribute("aria-pressed", "true");
  await session.send("Input.dispatchTouchEvent", {
    type: "touchMove",
    touchPoints: [{ x: to.x + 16, y: to.y + 16 }],
  });
  await expect(page.getByText("目标位置 2", { exact: true })).toBeAttached();
  await session.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await expect.poll(() => writes).toEqual([["gemini", "openai", "claude"]]);
  const row = page.getByRole("row").filter({ has: source });
  await expect(row).toHaveCSS("opacity", "1");
  await expect(row).toHaveCSS("transition-duration", "0s");
  await session.detach();
});

test("松手退出拖动的同一帧已采用新顺序，不先回原位再交换", async ({ page }) => {
  await setupDictionary(page);
  const source = page.getByRole("button", { name: "拖动openai", exact: true });
  await source.focus();
  await page.keyboard.press("Space");
  await expect(source).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("ArrowDown");
  await expect(page.getByText("目标位置 2", { exact: true })).toBeAttached();
  const landedOrder = page.evaluate(
    () =>
      new Promise<string[]>((resolve) => {
        const handle = document.querySelector('[aria-label="拖动openai"]')!;
        const body = handle.closest("tbody")!;
        const observer = new MutationObserver(() => {
          if (handle.getAttribute("aria-pressed") !== "false") return;
          observer.disconnect();
          resolve(Array.from(body.rows, (row) => row.cells[2].textContent ?? ""));
        });
        observer.observe(body, { subtree: true, childList: true, attributes: true });
      }),
  );
  await page.keyboard.press("Space");
  expect(await landedOrder).toEqual(["gemini", "openai", "claude"]);
});

test("第一行拖到第二行后不重播入场动画，拖拽透明度由排序组件控制", async ({ page }) => {
  await page.setViewportSize({ width: page.viewportSize()!.width, height: 900 });
  await setupDictionary(page);
  const source = page.getByRole("button", { name: "拖动openai", exact: true });
  const target = page.getByRole("button", { name: "拖动gemini", exact: true });
  const sourceRow = page.getByRole("row").filter({ has: source });
  await source.hover();
  const to = (await target.boundingBox())!;
  await page.mouse.down();
  await page.mouse.move(to.x + 16, to.y + 16, { steps: 8 });
  await expect(source).toHaveAttribute("aria-pressed", "true");
  await expect(sourceRow).toHaveCSS("opacity", "0.4");
  await page.mouse.up();
  await expect(page.getByRole("row").nth(2)).toContainText("openai");
  await expect(sourceRow).toHaveCSS("animation-name", "none");
  await expect(page.getByRole("row").filter({ has: target })).toHaveCSS("animation-name", "none");
  await expect(sourceRow).toHaveCSS("opacity", "1");
});

test("松手归位时列表保持可见，浮层淡出后可以继续排序", async ({ page }) => {
  const writes = await setupDictionary(page);
  await page.evaluate(() => {
    const animate = Element.prototype.animate;
    Element.prototype.animate = function (frames, options) {
      const animation = animate.call(this, frames, options);
      animation.pause();
      return animation;
    };
  });
  const source = page.getByRole("button", { name: "拖动openai", exact: true });
  await source.focus();
  await page.keyboard.press("Space");
  await expect(source).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("ArrowDown");
  await expect(page.getByText("目标位置 2", { exact: true })).toBeAttached();
  await page.keyboard.press("Space");
  await expect(page.getByRole("row").nth(2)).toContainText("openai");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          document.getAnimations().filter((animation) => animation.playState === "paused").length,
      ),
    )
    .toBeGreaterThan(0);
  const row = page.getByRole("row").filter({ has: source });
  expect(await row.evaluate((node) => Number(getComputedStyle(node).opacity))).toBeGreaterThan(0);
  const fade = await page.evaluate(() =>
    document.getAnimations().some((animation) => {
      if (!(animation.effect instanceof KeyframeEffect)) return false;
      return (
        animation.effect.target?.textContent === "openai" &&
        animation.effect.getKeyframes().at(-1)?.opacity === "0"
      );
    }),
  );
  expect(fade).toBe(true);
  await page.evaluate(() => {
    for (const animation of document.getAnimations()) animation.finish();
  });
  await expect(row).toHaveCSS("opacity", "1");
  await expect(source).toBeEnabled();
  await source.focus();
  await page.keyboard.press("Space");
  await expect(source).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("ArrowDown");
  await expect(page.getByText("目标位置 3", { exact: true })).toBeAttached();
  await page.keyboard.press("Space");
  await expect(page.getByRole("row").nth(3)).toContainText("openai");
  await expect
    .poll(() => writes)
    .toEqual([
      ["gemini", "openai", "claude"],
      ["gemini", "claude", "openai"],
    ]);
});
