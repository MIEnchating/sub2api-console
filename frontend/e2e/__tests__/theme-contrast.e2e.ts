import { expect, test } from "@playwright/test";

test("默认主题的状态文字在页面、面板和灰色选中背景上达到正文对比度", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", (route) =>
    route.fulfill({
      json:
        new URL(route.request().url()).pathname === "/api/setup/status"
          ? { initialized: true, configuration_errors: [] }
          : { authenticated: false },
    }),
  );
  await page.goto("/");
  await expect(page.getByRole("button", { name: "登录", exact: true })).toBeVisible();
  const contrasts = await page.evaluate(() => {
    const context = document.createElement("canvas").getContext("2d")!;
    const theme = getComputedStyle(document.documentElement);
    function luminance(token: string): number {
      context.fillStyle = theme.getPropertyValue(`--${token}`);
      context.fillRect(0, 0, 1, 1);
      return Array.from(context.getImageData(0, 0, 1, 1).data)
        .slice(0, 3)
        .map((value) => value / 255)
        .map((value) => (value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4))
        .reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
    }
    return ["background", "card", "muted"].flatMap((background) =>
      ["foreground", "muted-foreground", "success", "warning", "destructive", "info"].map(
        (foreground) => {
          const first = luminance(background);
          const second = luminance(foreground);
          return {
            background,
            foreground,
            ratio: (Math.max(first, second) + 0.05) / (Math.min(first, second) + 0.05),
          };
        },
      ),
    );
  });
  expect(contrasts.filter((entry) => entry.ratio < 4.5)).toEqual([]);
});
