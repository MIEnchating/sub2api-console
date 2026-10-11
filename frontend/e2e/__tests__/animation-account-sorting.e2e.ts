import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("动画与前置检测展示优先级和权重，排序跨标签保留且窄屏不溢出", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const rows = [
    {
      ...account,
      id: "1",
      name: "手动账号 · " + "长名称".repeat(12),
      platform: "openai",
      priority: 10,
      manual_priority: 1,
      weight: 2.5,
    },
    { ...account, id: "2", name: "自动账号", platform: "openai", priority: 2, weight: 10 },
    { ...account, id: "3", name: "缺失参数账号", platform: "openai", priority: null, weight: null },
  ];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "账号排序测试" },
      "/api/accounts": rows,
      "/api/model-checks/animations": [],
      "/api/model-checks/animation-schedules": [],
      "/api/dictionaries": { items: [] },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  const sort = page.getByRole("combobox", { name: "账号排序" });
  const cards = page.getByRole("article");
  for (const tab of ["账号检测", "前置检测"]) {
    await page.getByRole("tab", { name: tab, exact: true }).click();
    const manual = page.getByRole("article", { name: `账号 ${rows[0].name}`, exact: true });
    const metrics = manual.getByRole("group", { name: "账号调度参数" });
    const name = manual.getByText(rows[0].name, { exact: true });
    const nameBox = (await name.boundingBox())!;
    const metricsBox = (await metrics.boundingBox())!;
    expect(metricsBox.x).toBeGreaterThan(nameBox.x);
    expect(
      Math.abs(metricsBox.y + metricsBox.height / 2 - nameBox.y - nameBox.height / 2),
    ).toBeLessThan(2);
    await expect(manual.locator("header")).toHaveCSS("height", "72px");
    await expect(metrics.getByText("10", { exact: true })).toBeVisible();
    await expect(metrics.getByText("2.5", { exact: true })).toBeVisible();
    await sort.click();
    await page.getByRole("option", { name: "优先级：从小到大", exact: true }).click();
    await expect(cards.first()).toHaveAccessibleName("账号 自动账号");
    await expect(cards.last()).toHaveAccessibleName("账号 缺失参数账号");
    await sort.focus();
    await page.keyboard.press("Enter");
    await page.getByRole("option", { name: "权重：从大到小", exact: true }).click();
    await expect(cards.first()).toHaveAccessibleName("账号 自动账号");
    await expect(cards.last()).toHaveAccessibleName("账号 缺失参数账号");
    await sort.scrollIntoViewIfNeeded();
    expect(
      await sort.evaluate((element) => element.getBoundingClientRect().right <= innerWidth),
    ).toBe(true);
    for (const card of await cards.all()) {
      expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
        true,
      );
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: testInfo.outputPath(`${tab}-account-sorting.png`) });
  }
  await page.getByRole("tab", { name: "账号检测", exact: true }).click();
  await expect(sort).toContainText("权重：从大到小");
  await expect(cards.first()).toHaveAccessibleName("账号 自动账号");
  await page.getByRole("button", { name: "重置筛选" }).click();
  await expect(sort).toContainText("默认排序");
  await expect(cards.first()).toHaveAccessibleName(`账号 ${rows[0].name}`);
});
