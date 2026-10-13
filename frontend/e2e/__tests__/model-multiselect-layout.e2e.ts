import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("模型多选保持单行工具栏，搜索勾选与手机布局不溢出", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "dark"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/accounts": [{ id: "41", name: "测试账号", groups: [], platform: "openai" }],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [],
      "/api/model-checks/animations/accounts/41/models": {
        models: ["gpt-6-astra", "gpt-6.1-sol", "claude-sonnet-4-5"],
      },
      "/api/dictionaries": { items: [] },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("checkbox", { name: /检测 测试账号/ }).check();
  const trigger = page.getByRole("combobox", { name: "检测模型", exact: true });
  const toolbar = page.getByRole("group", { name: "动画检测操作", exact: true });
  const before = (await toolbar.boundingBox())!;
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
  await expect(page.getByRole("button", { name: "获取模型", exact: true })).toBeEnabled();
  await trigger.click();
  await page.getByRole("option", { name: "gpt-6.1-sol", exact: true }).click();
  await expect(page.getByRole("option", { name: "gpt-6.1-sol", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByRole("listbox")).toBeHidden();
  await expect(trigger).toContainText("gpt-6.1-sol");
  await expect(trigger).not.toContainText("+1");
  expect(
    await trigger
      .getByText("gpt-6-astra、gpt-6.1-sol", { exact: true })
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("two-models.png") });
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await page.getByRole("option", { name: "claude-sonnet-4-5", exact: true }).click();
  await page.screenshot({ path: testInfo.outputPath("models-open.png") });
  await page.keyboard.press("Escape");
  await expect(trigger).toContainText("+1");
  await expect(trigger).toHaveCSS("height", "32px");
  expect((await toolbar.boundingBox())!.height).toBe(before.height);
  await expect(page.getByRole("button", { name: "添加模型", exact: true })).toHaveCount(0);
  await trigger.click();
  await page.getByRole("combobox", { name: "搜索或输入模型 ID" }).fill("manual-model");
  await page.getByRole("option", { name: "使用模型 manual-model", exact: true }).click();
  await page.keyboard.press("Escape");
  await expect(trigger).toContainText("+2");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("models-collapsed.png") });
});
