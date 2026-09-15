import { expect, test, type Page, type Route } from "@playwright/test";

async function prepareLogin(
  page: Page,
  theme: "light" | "dark",
  onLogin?: (route: Route) => Promise<void>,
): Promise<void> {
  await page.addInitScript((initialTheme) => {
    if (!localStorage.getItem("sub2api-console-theme")) {
      localStorage.setItem("sub2api-console-theme", initialTheme);
    }
  }, theme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/setup/status") {
      await route.fulfill({ json: { initialized: true, configuration_errors: [] } });
    } else if (path === "/api/auth/session") {
      await route.fulfill({ json: { authenticated: false, username: null } });
    } else if (path === "/api/auth/login" && onLogin) {
      await onLogin(route);
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "登录", exact: true })).toBeVisible();
}

for (const compact of [false, true]) {
  test(`${compact ? "320 像素窄屏" : "常规视口"}登录页完整显示品牌且表单可滚动访问`, async ({
    page,
    colorScheme,
  }) => {
    if (compact) await page.setViewportSize({ width: 320, height: 480 });
    await prepareLogin(page, colorScheme === "light" ? "light" : "dark");

    const logo = page.locator('header img[src="/console-mark.svg"]').first();
    await expect(logo).toBeVisible();
    await expect(logo).toBeInViewport({ ratio: 1 });
    await expect
      .poll(() => logo.evaluate((node: HTMLImageElement) => node.complete && node.naturalWidth > 0))
      .toBe(true);
    const loginRegion = page.getByRole("region", { name: "登录", exact: true });
    const introduction = page.getByRole("complementary", { name: "控制台介绍", exact: true });
    await expect(page.getByText("Sub2API Console", { exact: true })).toHaveCount(1);
    await expect(
      introduction.getByRole("heading", { name: "Sub2API Console", exact: true }),
    ).toBeVisible();
    if ((page.viewportSize()?.width ?? 0) >= 1024) {
      await expect(introduction).toBeVisible();
      const introductionBounds = await introduction.boundingBox();
      const loginBounds = await loginRegion.boundingBox();
      expect(introductionBounds).not.toBeNull();
      expect(loginBounds).not.toBeNull();
      expect(introductionBounds!.x + introductionBounds!.width).toBeLessThanOrEqual(loginBounds!.x);
    } else {
      await expect(introduction).toBeVisible();
      const introductionBounds = await introduction.boundingBox();
      const loginBounds = await loginRegion.boundingBox();
      expect(introductionBounds!.y + introductionBounds!.height).toBeLessThanOrEqual(
        loginBounds!.y,
      );
    }
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    const username = page.getByRole("textbox", { name: "账号", exact: true });
    const password = page.getByLabel("密码", { exact: true });
    const submit = page.getByRole("button", { name: "登录", exact: true });
    if (!compact) await expect(submit).toBeInViewport({ ratio: 1 });
    for (const control of [username, password, submit]) {
      await control.scrollIntoViewIfNeeded();
      await expect(control).toBeInViewport({ ratio: 1 });
    }
    await expect(username).toHaveAttribute("autocomplete", "username");
    await expect(username).toHaveAttribute("autocapitalize", "none");
    await expect(username).toHaveAttribute("spellcheck", "false");
    await expect(password).toHaveAttribute("autocomplete", "current-password");
    await expect(submit).toHaveCSS("height", "32px");
    if (compact) {
      const documentScroll = await page.evaluate(() => ({
        scrollable: document.documentElement.scrollHeight > window.innerHeight,
        position: window.scrollY,
      }));
      if (documentScroll.scrollable) expect(documentScroll.position).toBeGreaterThan(0);
    }
    await page.screenshot({
      path: test.info().outputPath(compact ? "login-320.png" : "login.png"),
      fullPage: true,
      animations: "disabled",
    });
  });
}

test("宽屏登录页展开品牌与等比关系图，低高度时表单仍可滚动访问", async ({ page, colorScheme }) => {
  await page.setViewportSize({ width: 1920, height: 959 });
  await prepareLogin(page, colorScheme === "light" ? "light" : "dark");
  await expect(page.getByRole("main")).toHaveCSS("max-width", "1440px");
  const illustration = page.getByRole("img", { name: /^Console 统一管理 Sub2API/ });
  const diagramBounds = await illustration.boundingBox();
  expect(diagramBounds!.width).toBe(800);
  expect(diagramBounds!.width / diagramBounds!.height).toBeCloseTo(680 / 380, 2);
  const form = page.getByRole("region", { name: "登录", exact: true });
  expect((await form.boundingBox())!.width).toBe(360);
  await expect(page.getByRole("heading", { name: "Sub2API Console", exact: true })).toHaveCSS(
    "font-size",
    "40px",
  );
  await page.screenshot({ path: test.info().outputPath("login-wide.png"), fullPage: true });

  await page.setViewportSize({ width: 1920, height: 600 });
  await page.getByRole("button", { name: "登录", exact: true }).scrollIntoViewIfNeeded();
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.setViewportSize({ width: 1024, height: 768 });
  await expect(illustration).toBeInViewport({ ratio: 1 });
  await expect(form).toBeInViewport({ ratio: 1 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(1024);
});

test("密码显隐可通过键盘切换且回车提交保留原始密码", async ({ page, colorScheme }) => {
  let submitted: unknown;
  await prepareLogin(page, colorScheme === "light" ? "light" : "dark", async (route) => {
    submitted = route.request().postDataJSON();
    await route.fulfill({ status: 503, json: { detail: "登录服务暂不可用，请重试" } });
  });
  const username = page.getByRole("textbox", { name: "账号", exact: true });
  const password = page.getByLabel("密码", { exact: true });
  await username.fill("operator");
  await username.press("Tab");
  await expect(password).toBeFocused();
  await password.fill("login-fixture-password");
  await password.press("Tab");
  const reveal = page.getByRole("button", { name: "显示密码", exact: true });
  await expect(reveal).toBeFocused();
  await expect(reveal).toHaveAttribute("aria-pressed", "false");
  await reveal.press("Enter");
  await expect(password).toHaveAttribute("type", "text");
  const conceal = page.getByRole("button", { name: "隐藏密码", exact: true });
  await expect(conceal).toBeFocused();
  await expect(conceal).toHaveAttribute("aria-pressed", "true");
  await conceal.press("Space");
  await expect(password).toHaveAttribute("type", "password");
  await password.focus();
  await password.press("Enter");

  await expect(page.getByText("登录服务暂不可用，请重试", { exact: true })).toBeVisible();
  expect(submitted).toEqual({ username: "operator", password: "login-fixture-password" });
});

test("登录等待时锁定表单并保持按钮尺寸，失败后保留输入供重试", async ({ page, colorScheme }) => {
  let release: () => void = () => {};
  const responseReady = new Promise<void>((resolve) => {
    release = resolve;
  });
  let attempts = 0;
  await prepareLogin(page, colorScheme === "light" ? "light" : "dark", async (route) => {
    attempts += 1;
    if (attempts === 1) await responseReady;
    await route.fulfill({ status: 503, json: { detail: "登录验证暂不可用，请稍后重试" } });
  });
  const username = page.getByRole("textbox", { name: "账号", exact: true });
  const password = page.getByLabel("密码", { exact: true });
  const reveal = page.getByRole("button", { name: "显示密码", exact: true });
  const submit = page.getByRole("button", { name: "登录", exact: true });
  await username.fill("operator");
  await password.fill("login-fixture-password");
  const initialBounds = await submit.boundingBox();
  await submit.click();
  const pending = page.getByRole("button", { name: "登录中…", exact: true });
  await expect(pending).toBeDisabled();
  await expect(pending).toHaveAttribute("aria-busy", "true");
  await expect(username).toBeDisabled();
  await expect(password).toBeDisabled();
  await expect(reveal).toBeDisabled();
  expect(await pending.boundingBox()).toEqual(initialBounds);
  await page.keyboard.press("Enter");
  expect(attempts).toBe(1);
  release();

  await expect(page.getByText("登录验证暂不可用，请稍后重试", { exact: true })).toBeVisible();
  await expect(submit).toBeEnabled();
  await expect(username).toBeEditable();
  await expect(password).toBeEditable();
  await expect(reveal).toBeEnabled();
  await expect(username).toHaveValue("operator");
  await expect(password).toHaveValue("login-fixture-password");
  expect(await submit.boundingBox()).toEqual(initialBounds);
  await submit.click();
  await expect.poll(() => attempts).toBe(2);
  await expect(submit).toBeEnabled();
});

test("登录前切换主题后页面与控件同步且刷新保留选择", async ({ page, colorScheme }) => {
  const initialTheme = colorScheme === "light" ? "light" : "dark";
  const nextTheme = initialTheme === "light" ? "dark" : "light";
  await prepareLogin(page, initialTheme);
  const toggleName = initialTheme === "light" ? "切换暗色主题" : "切换亮色主题";
  const nextToggleName = initialTheme === "light" ? "切换亮色主题" : "切换暗色主题";
  await page.getByRole("button", { name: toggleName, exact: true }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", nextTheme);
  await expect(page.getByRole("button", { name: nextToggleName, exact: true })).toBeVisible();
  await page.reload();

  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveAttribute("data-theme", nextTheme);
  await expect(page.getByRole("button", { name: nextToggleName, exact: true })).toBeVisible();
});

test("浏览器报告离线时登录仍尝试本地接口，失败后恢复可重试表单", async ({ page, colorScheme }) => {
  let attempts = 0;
  await prepareLogin(page, colorScheme === "light" ? "light" : "dark", async (route) => {
    attempts += 1;
    await route.fulfill({ status: 503, json: { detail: "登录服务暂不可用，请重试" } });
  });
  const username = page.getByRole("textbox", { name: "账号", exact: true });
  const password = page.getByLabel("密码", { exact: true });
  await username.fill("operator");
  await password.fill("login-fixture-password");
  // A browser offline signal must not queue credentials when the console's
  // local endpoint may remain reachable. Keep assets and the mock API online.
  await page.evaluate(() => window.dispatchEvent(new Event("offline")));
  await page.getByRole("button", { name: "登录", exact: true }).click();

  await expect(page.getByText("登录服务暂不可用，请重试", { exact: true })).toBeVisible();
  expect(attempts).toBe(1);
  await expect(username).toBeEditable();
  await expect(password).toBeEditable();
  await expect(page.getByRole("button", { name: "显示密码", exact: true })).toBeEnabled();
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeEnabled();
});
