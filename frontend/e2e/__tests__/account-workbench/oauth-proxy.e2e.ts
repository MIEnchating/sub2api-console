import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("人工授权代理保持私有输入并支持键盘启动，结束后清空代理", async ({ page }) => {
  await installWorkbenchFixture(page);
  const starts: unknown[] = [];
  await page.route("**/api/account-workbench/oauth", async (route) => {
    starts.push(route.request().postDataJSON() as unknown);
    await route.fallback();
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  const proxy = page.getByLabel("登录代理", { exact: true });
  await expect(proxy).toHaveAttribute("type", "password");
  await proxy.fill("https://username:fixture-password@proxy.example.test:443");
  const start = page.getByRole("button", { name: "开始授权登录" });
  await expect(start).toBeInViewport();
  expect(
    await page
      .getByRole("region", { name: "账号授权登录" })
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  await start.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button", { name: "上游登录页面" })).toBeVisible();
  expect(starts).toEqual([
    { proxy_url: "https://username:fixture-password@proxy.example.test:443" },
  ]);
  await page.getByRole("button", { name: "结束授权" }).click();
  await expect(proxy).toHaveValue("");
});
