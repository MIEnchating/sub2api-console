import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("上架获取模型支持搜索、键盘选择和窄屏滚动，确认后保留手输模型", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const longModel = "vendor/" + "long-model-name-".repeat(12);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "渠道测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/channels": {
        items: [
          {
            id: "42",
            name: "标准渠道",
            type: 59,
            status: 1,
            models: ["old-model"],
            groups: ["default"],
            version: "a".repeat(64),
          },
        ],
        total: 1,
      },
      "/api/newapi/platforms/layout/channels/42/models/available": {
        models: [
          "old-model",
          longModel,
          ...Array.from({ length: 40 }, (_, index) => `new-model-${index}`),
        ],
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": test\n\n" });
    else if (path.startsWith("/api/dictionaries")) await route.fulfill({ json: { items: [] } });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "未配置隔离测试接口" } });
  });
  await page.goto("/newapi/channels");
  await page.getByRole("button", { name: "上架模型", exact: true }).click();
  await page.getByLabel("上架模型名称").fill("manual-model");
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
  const picker = page.getByRole("dialog", { name: "选择上游模型" });
  await expect(picker.getByRole("checkbox")).toHaveCount(41);
  await expect(picker.getByRole("list", { name: "上游模型" })).toHaveCSS("overflow-y", "auto");
  expect(await picker.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(picker.getByRole("button", { name: "确认模型" })).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("channel-models.png") });
  await picker.getByRole("textbox", { name: "搜索上游模型" }).fill("vendor/");
  const choice = picker.getByRole("checkbox");
  await choice.focus();
  await page.keyboard.press("Space");
  await expect(choice).toBeChecked();
  await picker.getByRole("button", { name: "确认模型" }).click();
  await expect(page.getByLabel("上架模型名称")).toHaveValue(`manual-model\n${longModel}`);
  await page.getByRole("button", { name: "预览变更" }).click();
  await expect(page.getByRole("list", { name: "变更模型" })).toContainText(longModel);
});
