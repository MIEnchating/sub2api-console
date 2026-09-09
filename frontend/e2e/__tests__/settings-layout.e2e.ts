import { expect, test } from "@playwright/test";

import { policy, pricing } from "./fixtures/settings";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/policy": policy,
      "/api/pricing": pricing,
      "/api/config": { probes_enabled: true },
      "/api/accounts": [],
      "/api/groups": [],
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (route.request().method() === "GET" && path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("策略分类完整显示，键盘切换后保留草稿且滚动时导航与保存可见", async ({ page }) => {
  await page.goto("/policy");
  const tabs = page.getByRole("tablist", { name: "策略分类" });
  await expect(tabs.getByRole("tab")).toHaveCount(4);
  for (const tab of await tabs.getByRole("tab").all()) {
    await expect(tab).toBeInViewport({ ratio: 1 });
  }
  const budget = page.getByRole("spinbutton", { name: "每组总权重预算", exact: true });
  await budget.fill("500");
  const routing = tabs.getByRole("tab", { name: "调度与写入" });
  await routing.focus();
  await page.keyboard.press("ArrowRight");
  await expect(tabs.getByRole("tab", { name: "健康与处置" })).toBeFocused();
  await expect(page.getByRole("tabpanel", { name: "健康与处置" })).toBeVisible();
  await page.keyboard.press("Home");
  await expect(budget).toHaveValue("500");
  await page.getByText("查看权重计算说明", { exact: true }).click();
  await expect(page.getByText(/最终权重 = 组内预算/)).toBeVisible();
  const content = page.locator('[data-slot="page-content"]');
  await content.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(tabs).toBeInViewport({ ratio: 1 });
  await expect(page.getByRole("button", { name: "保存策略", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await content.evaluate((element) => element.scrollTo(0, 0));
  await page.getByText("查看权重计算说明", { exact: true }).click();
  await page.screenshot({ path: test.info().outputPath("policy.png") });
});

test("价格配置宽屏双列窄屏堆叠，选择和折叠互换组不丢失设置", async ({ page, viewport }) => {
  await page.goto("/pricing-config");
  const settings = page.getByRole("region", { name: "自动价格分组" });
  const exchange = page.getByRole("region", { name: "账号互换范围" });
  await expect(settings).toBeVisible();
  const left = (await settings.boundingBox())!;
  const right = (await exchange.boundingBox())!;
  if (viewport!.width >= 1280) {
    expect(right.x).toBeGreaterThanOrEqual(left.x + left.width);
    expect(Math.abs(right.y - left.y)).toBeLessThanOrEqual(4);
  } else {
    expect(right.y).toBeGreaterThanOrEqual(left.y + left.height);
    await expect(page.getByText("账号互换范围")).toBeInViewport({ ratio: 1 });
  }
  const option = page.getByRole("checkbox", { name: /^互换组 1 分组 codex-pro/ });
  await option.check();
  await page.getByRole("button", { name: "收起互换组 1" }).click();
  await expect(option).not.toBeVisible();
  await page.getByRole("button", { name: "展开互换组 1" }).click();
  await expect(option).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "互换组 1 分组 暂不可用" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "保存配置", exact: true })).toBeInViewport({
    ratio: 1,
  });
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await content.evaluate((element) => element.scrollTo(0, 0));
  await page.screenshot({ path: test.info().outputPath("pricing-config.png") });
});

test("价格规则可用键盘展开，自动调整开关保留已填写参数", async ({ page }) => {
  await page.goto("/pricing-config");
  const margin = page.getByRole("spinbutton", { name: "目标盈利比例", exact: true });
  await margin.fill("25");
  const toggle = page.getByRole("switch", { name: "启用动态价格分组" });
  await toggle.focus();
  await page.keyboard.press("Space");
  await expect(toggle).toBeChecked();
  await expect(margin).toHaveValue("25");
  const description = page.getByText(/均亏损时保留当前分组/);
  await expect(description).not.toBeVisible();
  await page.getByText("查看分组选择规则", { exact: true }).focus();
  await page.keyboard.press("Enter");
  await expect(description).toBeVisible();
});

test("超长分组名完整换行且互换组操作不超出页面", async ({ page }) => {
  const name = "生产环境专用分组".repeat(15);
  await page.route("**/api/pricing", (route) =>
    route.fulfill({
      json: {
        ...pricing,
        groups: [{ ...pricing.groups[0], name }, ...pricing.groups.slice(1)],
      },
    }),
  );
  await page.goto("/pricing-config");
  const label = page.getByText(name, { exact: true });
  await expect(label).toBeVisible();
  expect(await label.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.getByRole("button", { name: "删除互换组 1" }).click();
  await expect(page.getByText("还没有互换组")).toBeVisible();
  await page.getByRole("button", { name: "创建第一个互换组" }).click();
  await expect(page.getByRole("textbox", { name: "互换组 1 规则名称" })).toBeVisible();
});
