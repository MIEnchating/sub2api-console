import { expect, test } from "@playwright/test";
import { pageFixtures, workspace } from "./fixtures/page-shell";

const groupName = "渠道布局验证专用分组名称".repeat(5);
const endpoint = `https://api.example.test/${"long-api-path/".repeat(8)}`;

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "渠道布局测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi": { ...workspace, local_groups: [{ id: "6", name: groupName, ratio: "1" }] },
      "/api/dictionaries": { items: [{ value: "6", enabled: true }] },
      "/api/newapi/platforms/layout/channel-key": {
        key_id: "layout-key",
        name: groupName,
        group_id: "6",
        endpoints: [{ name: "测试接口", base_url: endpoint, default: true }],
      },
      "/api/newapi/platforms/layout/channel-models": { models: ["test-model"] },
      "/api/newapi/platforms/layout/channels": { id: "layout-channel" },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

test("渠道表单随容器分栏，长名称与地址不撑宽页面，两步提交操作均可达", async ({ page }, info) => {
  await page.goto("/newapi/channels");
  await page.getByRole("button", { name: "自定义账号密码" }).click();
  const layout = page.locator("[data-channel-credentials-layout]");
  const columns = await layout.evaluate((element) =>
    getComputedStyle(element).gridTemplateColumns.split(" "),
  );
  const layoutWidth = await layout.evaluate((element) => element.getBoundingClientRect().width);
  expect(columns.length).toBe(layoutWidth >= 768 ? 2 : 1);
  await page.getByRole("textbox", { name: "登录邮箱" }).fill("layout@example.test");
  await page.getByLabel("密码", { exact: true }).fill("isolated-test-password");
  await page.getByRole("combobox", { name: "Sub2API 分组", exact: true }).click();
  await page.getByRole("option", { name: groupName, exact: true }).click();
  await expect(page.getByRole("button", { name: "创建密钥", exact: true })).toBeEnabled();
  expect(
    await page.getByRole("combobox", { name: "Sub2API 分组", exact: true }).evaluate((element) => {
      const column = element.closest("fieldset")!;
      return element.getBoundingClientRect().right <= column.getBoundingClientRect().right;
    }),
  ).toBe(true);
  if (info.project.name === "desktop-light")
    await page.screenshot({ path: "/tmp/channel-credentials-desktop.png", fullPage: true });
  await page.getByRole("button", { name: "创建密钥", exact: true }).click();
  await expect(page.locator('[data-channel-step="configuration"]')).toHaveAttribute(
    "aria-current",
    "step",
  );
  await expect(page.getByText(groupName, { exact: true })).toHaveCSS("overflow-wrap", "anywhere");
  await expect(page.getByRole("combobox", { name: "API 地址来源" })).toBeVisible();
  expect(
    await page.getByRole("combobox", { name: "API 地址来源" }).evaluate((element) => {
      const column = element.parentElement!;
      return element.getBoundingClientRect().right <= column.getBoundingClientRect().right;
    }),
  ).toBe(true);
  await page.getByRole("combobox", { name: "New API 分组", exact: true }).click();
  await page.getByRole("option", { name: "默认", exact: true }).click();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "从上游获取", exact: true }).click();
  await page.getByRole("button", { name: "确认模型", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "已选择 1 个模型" })).toBeVisible();
  await page.getByRole("button", { name: "添加渠道", exact: true }).scrollIntoViewIfNeeded();
  await expect(page.getByRole("button", { name: "添加渠道", exact: true })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  if (info.project.name === "desktop-light")
    await page.screenshot({ path: "/tmp/channel-configuration-desktop.png", fullPage: true });
});

test("低高度窗口可滚动到底部且常规按钮保持32像素", async ({ page }) => {
  await page.setViewportSize({ width: 640, height: 420 });
  await page.goto("/newapi/channels");
  await page.getByRole("button", { name: "自定义账号密码" }).click();
  const submit = page.getByRole("button", { name: "创建密钥", exact: true });
  await submit.scrollIntoViewIfNeeded();
  await expect(submit).toBeInViewport();
  await expect(submit).toHaveCSS("height", "32px");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

test("默认账号表单与全局工作区对齐，宽屏账号和分组同行且面板保持紧凑", async ({ page }, info) => {
  await page.goto("/newapi/channels");
  await expect(page.getByRole("combobox", { name: "密码箱账号", exact: true })).toBeVisible();
  const form = page
    .locator('[data-slot="card"]')
    .filter({ has: page.locator("[data-channel-credentials-layout]") });
  const dimensions = await form.evaluate((element) => ({
    width: element.getBoundingClientRect().width,
    available: element.parentElement!.clientWidth,
  }));
  expect(dimensions.available - dimensions.width).toBeLessThanOrEqual(4);
  const account = await page
    .getByRole("combobox", { name: "密码箱账号", exact: true })
    .boundingBox();
  const group = await page
    .getByRole("combobox", { name: "Sub2API 分组", exact: true })
    .boundingBox();
  if (dimensions.width >= 768) {
    expect(account!.y).toBe(group!.y);
    expect(Math.abs(account!.width - group!.width)).toBeLessThanOrEqual(1);
  } else {
    expect(group!.y).toBeGreaterThan(account!.y + account!.height);
  }
  expect(await form.evaluate((element) => element.getBoundingClientRect().height)).toBeLessThan(
    620,
  );
  await page.screenshot({ path: `/tmp/channel-rework-${info.project.name}.png`, fullPage: true });
});
