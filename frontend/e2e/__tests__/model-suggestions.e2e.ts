import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("模型建议跟随主题和输入宽度，长名称与滚动列表不溢出视口", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  const longModel = "long-model-".repeat(20);
  const fixtures: Record<string, unknown> = {
    ...pageFixtures,
    "/api/setup/status": { initialized: true, configuration_errors: [] },
    "/api/auth/session": { authenticated: true, username: "样式测试" },
    "/api/accounts": [{ ...account, platform: "openai" }],
    "/api/model-checks/animation-schedules": [],
    "/api/model-checks/animations": [],
    "/api/model-checks/animations/accounts/41/models": {
      models: ["codex-auto-review", "gpt-5.5", "gpt-5.6-sol", "gpt-6-astra", longModel],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  await page.goto("/model-check");
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  await page.getByRole("checkbox", { name: /检测 待校验账号/ }).check();
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
  const input = page.getByRole("combobox", { name: "检测模型", exact: true });
  await input.click();
  await expect(page.getByRole("option", { name: "gpt-6-astra", exact: true })).toBeVisible();
  const popup = page.locator('[data-slot="combobox-content"]');
  await expect(popup).toBeInViewport({ ratio: 1 });
  await expect(input).toHaveCSS("height", "32px");
  await expect(popup).toHaveCSS("border-radius", "8px");
  const geometry = await popup.evaluate((element) => {
    const inputElement = document.querySelector<HTMLInputElement>("#animation-unified-model")!;
    const themeProbe = document.createElement("div");
    themeProbe.style.backgroundColor = "var(--popover)";
    document.body.append(themeProbe);
    const themeColor = getComputedStyle(themeProbe).backgroundColor;
    themeProbe.remove();
    return {
      width: element.getBoundingClientRect().width,
      inputWidth: inputElement.getBoundingClientRect().width,
      overflowing: element.scrollWidth > element.clientWidth,
      color: getComputedStyle(element).backgroundColor,
      themeColor,
    };
  });
  expect(geometry.width).toBe(geometry.inputWidth);
  expect(geometry.overflowing).toBe(false);
  expect(geometry.color).toBe(geometry.themeColor);
  await expect(page.getByText(longModel, { exact: true })).toHaveCSS("overflow-wrap", "anywhere");
  const list = page.getByRole("listbox");
  await expect(list).toHaveCSS("overflow-y", "auto");
  await expect(list).toHaveCSS("max-height", "288px");
  await page.screenshot({ path: testInfo.outputPath("model-suggestions.png") });
  await page.getByRole("option", { name: "gpt-6-astra", exact: true }).click();
  await expect(input).toHaveValue("gpt-6-astra");
  await expect(input).toHaveAttribute("aria-expanded", "false");
  await input.fill("custom-model");
  await page.locator("#animation-timeout").click();
  await expect(page.getByRole("spinbutton", { name: "请求超时（秒）" })).toBeFocused();
  await expect(input).toHaveValue("custom-model");
  await expect(input).toHaveAttribute("aria-expanded", "false");
});
