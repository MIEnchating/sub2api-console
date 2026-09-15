import { expect, test, type Page } from "@playwright/test";

const illustrationName = "Console 统一管理 Sub2API 的账号分组、健康监测、倍率同步与自动调度";

async function openLogin(page: Page, theme: string | null): Promise<void> {
  await page.addInitScript(
    (value) => localStorage.setItem("sub2api-console-theme", value ?? "light"),
    theme,
  );
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/setup/status")
      return route.fulfill({ json: { initialized: true, configuration_errors: [] } });
    if (path === "/api/auth/session") return route.fulfill({ json: { authenticated: false } });
    return route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/");
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeVisible();
}

test("桌面和手机均完整展示 Console 接管 Sub2API 的管理关系", async ({ page, colorScheme }) => {
  await openLogin(page, colorScheme);
  const illustration = page.getByRole("img", { name: illustrationName, exact: true });
  await expect(illustration).toHaveCount(1);
  await expect(illustration).toBeInViewport({ ratio: 1 });
  const illustrationBounds = await illustration.boundingBox();
  const formBounds = await page.getByRole("region", { name: "登录", exact: true }).boundingBox();
  if ((page.viewportSize()?.width ?? 0) < 1024) {
    expect(illustrationBounds!.y + illustrationBounds!.height).toBeLessThanOrEqual(formBounds!.y);
    for (const label of ["账号分组", "健康监测", "倍率同步", "自动调度"]) {
      const caption = illustration.getByText(label, { exact: true });
      await expect(caption).toHaveCSS("font-size", "12px");
      await expect(caption).toBeInViewport({ ratio: 1 });
    }
  } else {
    expect(illustrationBounds!.x + illustrationBounds!.width).toBeLessThan(formBounds!.x);
  }
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.screenshot({
    path: test.info().outputPath("login-orchestration.png"),
    fullPage: true,
  });
});

test("连接信号自行播放且鼠标移动不会改变相同时刻的画面", async ({ page, colorScheme }) => {
  await openLogin(page, colorScheme);
  const illustration = page.getByRole("img", { name: illustrationName, exact: true });
  const signal = illustration.locator(".login-map-signals path").first();
  const initial = await signal.evaluate((element) => getComputedStyle(element).strokeDashoffset);
  await expect
    .poll(() => signal.evaluate((element) => getComputedStyle(element).strokeDashoffset))
    .not.toBe(initial);
  await page.evaluate(() => {
    for (const animation of document.getAnimations()) animation.pause();
  });
  const stationary = await illustration.screenshot();
  await illustration.hover({ position: { x: 15, y: 15 } });
  expect(await illustration.screenshot()).toEqual(stationary);
});

test("减少动效偏好下管理关系静止显示且仍能填写账号", async ({ page, colorScheme }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await openLogin(page, colorScheme);
  const illustration = page.getByRole("img", { name: illustrationName, exact: true });
  await expect(illustration.locator(".login-map-signals path").first()).toHaveCSS(
    "animation-name",
    "none",
  );
  await page.getByRole("textbox", { name: "账号", exact: true }).fill("operator");
  await expect(page.getByRole("textbox", { name: "账号", exact: true })).toHaveValue("operator");
});

test("切换主题后品牌与管理关系保持可见且静态图标资源完整", async ({ page, colorScheme }) => {
  await openLogin(page, colorScheme);
  const toggle = colorScheme === "dark" ? "切换亮色主题" : "切换暗色主题";
  await page.getByRole("button", { name: toggle, exact: true }).click();
  await expect(page.getByRole("img", { name: illustrationName, exact: true })).toBeInViewport({
    ratio: 1,
  });
  const logo = page.locator('header img[src="/console-mark.svg"]');
  await expect
    .poll(() =>
      logo.evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0),
    )
    .toBe(true);
  await expect(page.locator('link[rel="icon"][type="image/svg+xml"]')).toHaveAttribute(
    "href",
    "/console-mark.svg",
  );
  const sizes = await page.evaluate(async () => {
    const results: number[] = [];
    for (const source of [
      "/favicon-32x32.png",
      "/apple-touch-icon.png",
      "/icon-192.png",
      "/icon-512.png",
    ]) {
      const image = new Image();
      image.src = source;
      await image.decode();
      results.push(image.naturalWidth, image.naturalHeight);
    }
    return results;
  });
  expect(sizes).toEqual([32, 32, 180, 180, 192, 192, 512, 512]);
});
