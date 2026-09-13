import { expect, test } from "@playwright/test";
import type { Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

const animationAccount = {
  ...account,
  name: "动画检测的超长账号名称".repeat(6),
  platform: "openai",
  groups: ["超长分组".repeat(30)],
  upstream_host: "https://" + "very-long-host".repeat(20) + ".example",
};
const task: Task = {
  id: "animation-e2e",
  skill: "sub2api-model-animation",
  operation: "account-model-animation",
  status: "succeeded",
  progress: 100,
  message: "动画检测完成：成功 1，失败 0",
  created_at: "2026-09-13T00:00:00Z",
  updated_at: "2026-09-13T00:00:00Z",
  result: {
    animations: [
      {
        account_id: "41",
        account_name: animationAccount.name,
        model: "fixture-model",
        response_model: "fixture-model",
        request_id: "animation-request",
        status: "succeeded",
        duration_ms: 1000,
        completed_at: "2026-09-13T00:00:00Z",
        svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><rect width="640" height="400" fill="#eef2ff"/><circle cx="320" cy="200" r="60" fill="#4338ca"><animate attributeName="r" values="60;80;60" dur="2s" repeatCount="indefinite"/></circle></svg>',
      },
    ],
  },
};

test("动画检测 Tab 在窄屏可滚动选择、确认费用并展示隔离动画", async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  let created = false;
  let currentTask: Task = {
    ...task,
    status: "running",
    progress: 0,
    message: "正在生成鹈鹕骑自行车动画",
    result: { account_ids: ["41"], animations: [] },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/animations" && route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({
        targets: [{ account_id: "41", model: "fixture-model" }],
        timeout_seconds: 120,
      });
      created = true;
      await route.fulfill({ json: currentTask });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "动画测试" },
      "/api/accounts": [animationAccount],
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": created ? [{ ...currentTask, result: {} }] : [],
      "/api/tasks/animation-e2e": currentTask,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/model-check");
  const entry = page.getByRole("tab", { name: "动画检测", exact: true });
  await entry.click();
  const dialog = page.getByRole("tabpanel", { name: "动画检测", exact: true });
  await expect(dialog).toBeVisible();
  const start = dialog.getByRole("button", { name: /开始检测/ });
  await expect(start).toBeInViewport({ ratio: 1 });
  await expect(start).toHaveCSS("height", "32px");
  await dialog.getByRole("checkbox", { name: /检测 动画检测/ }).check();
  await dialog.getByRole("combobox", { name: "检测模型" }).fill("fixture-model");
  const settings = dialog.getByRole("group", { name: "动画检测设置", exact: true });
  const region = dialog.getByRole("region", { name: "动画账号卡片", exact: true });
  const settingsHeight = (await settings.boundingBox())!.height;
  const regionTop = (await region.boundingBox())!.y;
  await start.click();
  const confirmation = page.getByRole("dialog", { name: "确认动画检测范围" });
  await expect(confirmation).toContainText("API 用量");
  expect(created).toBe(false);
  await confirmation.getByRole("button", { name: "确认并开始检测" }).click();
  const accountCard = dialog.getByRole("article", {
    name: "账号 " + animationAccount.name,
    exact: true,
  });
  await expect(dialog.getByRole("article")).toHaveCount(1);
  await expect(accountCard.getByRole("checkbox", { name: /检测 动画检测/ })).toBeChecked();
  await expect(accountCard.getByText("正在检测，等待动画结果", { exact: true })).toBeVisible();
  const operations = settings.getByRole("group", { name: "动画检测操作" });
  await expect(operations.getByRole("button", { name: "取消任务", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect((await settings.boundingBox())!.height).toBe(settingsHeight);
  expect((await region.boundingBox())!.y).toBe(regionTop);
  currentTask = { ...currentTask, progress: 50, message: "正在生成鹈鹕骑自行车动画，已完成一半" };
  await page.waitForResponse(
    async (response) =>
      response.url().endsWith("/api/tasks/animation-e2e") &&
      (await response.json()).progress === 50,
  );
  await expect(settings.getByText(/正在生成鹈鹕骑自行车动画/)).toHaveCount(0);
  expect((await settings.boundingBox())!.height).toBe(settingsHeight);
  expect((await region.boundingBox())!.y).toBe(regionTop);
  currentTask = task;
  const image = accountCard.getByRole("img", { name: /生成的鹈鹕骑自行车动画/ });
  await expect(image).toBeInViewport();
  expect((await settings.boundingBox())!.height).toBe(settingsHeight);
  expect((await region.boundingBox())!.y).toBe(regionTop);
  await expect(accountCard).toHaveCSS("height", "360px");
  await expect(accountCard.getByRole("button", { name: /收起动画|重新展示动画/ })).toHaveCount(0);
  expect((await image.boundingBox())!.height).toBeGreaterThanOrEqual(180);
  await expect(accountCard.locator("footer")).toHaveCSS("height", "44px");
  await expect(image).toHaveJSProperty("complete", true);
  expect(await image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBeGreaterThan(
    0,
  );
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: /开始检测/ })).toBeInViewport({ ratio: 1 });
  const originalSource = (await image.getAttribute("src"))!;
  const originalHeight = (await image.boundingBox())!.height;
  await image.click();
  const preview = page.getByRole("dialog", { name: "动画预览", exact: true });
  await expect(preview).toBeVisible();
  await expect(preview).toBeInViewport({ ratio: 1 });
  const enlarged = preview.getByRole("img", { name: /生成的鹈鹕骑自行车动画/ });
  await expect(enlarged).toHaveAttribute("src", originalSource);
  expect((await enlarged.boundingBox())!.height).toBeGreaterThan(originalHeight);
  await expect(preview.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.screenshot({ path: test.info().outputPath("animation-enlarged.png") });
  await page.keyboard.press("Escape");
  await expect(preview).not.toBeVisible();
  await expect(accountCard.getByRole("button", { name: /放大查看/ })).toBeFocused();
  await page.screenshot({ path: test.info().outputPath("model-animation.png") });
  await page.getByRole("tab", { name: "常规检测" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("tab", { name: "常规检测" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
});

test("大量账号时仅渲染当前页，跨页编辑和搜索后保留检测范围", async ({ page }) => {
  if ((page.viewportSize()?.width ?? 0) >= 1024)
    await page.setViewportSize({ width: 1100, height: 900 });
  const accounts = Array.from({ length: 240 }, (_, index) => ({
    ...account,
    id: String(index + 1),
    name: `批量账号 ${index + 1}`,
    platform: "openai",
    manual_priority: index === 0 ? 0 : null,
    groups: index === 0 ? ["主组"] : ["备用组"],
  }));
  const historyTask: Task = {
    ...task,
    result: {
      animations: [
        {
          account_id: "1",
          account_name: "批量账号 1",
          model: "saved-model",
          request_id: "saved-1",
          status: "succeeded",
          svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="20"/></svg>',
          duration_ms: 1000,
          completed_at: "2026-09-13T00:00:00Z",
        },
        {
          account_id: "2",
          account_name: "批量账号 2",
          model: "saved-model",
          request_id: "saved-2",
          status: "failed",
          error: "上游拒绝请求，请检查账号余额。".repeat(50),
          duration_ms: 1000,
          completed_at: "2026-09-13T00:00:00Z",
        },
      ],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "分页测试" },
      "/api/accounts": accounts,
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [{ ...historyTask, result: {} }],
      "/api/tasks/animation-e2e": historyTask,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/model-check");
  const entry = page.getByRole("tab", { name: "动画检测", exact: true });
  await entry.click();
  const dialog = page.getByRole("tabpanel", { name: "动画检测", exact: true });
  const cards = dialog.getByRole("checkbox", { name: /检测 批量账号/ });
  await expect(cards).toHaveCount(12);
  const accountCards = dialog.getByRole("article");
  const firstCardBox = (await accountCards.nth(0).boundingBox())!;
  if ((page.viewportSize()?.width ?? 0) >= 1024) {
    expect((await accountCards.nth(1).boundingBox())!.y).toBe(firstCardBox.y);
    expect((await accountCards.nth(2).boundingBox())!.y).toBe(firstCardBox.y);
    expect((await accountCards.nth(3).boundingBox())!.y).toBe(firstCardBox.y);
    expect((await accountCards.nth(4).boundingBox())!.y).toBeGreaterThan(firstCardBox.y);
  } else {
    expect((await accountCards.nth(1).boundingBox())!.y).toBeGreaterThan(firstCardBox.y);
  }

  const emptyCard = dialog.getByRole("article", { name: "账号 批量账号 3", exact: true });
  await expect(emptyCard.getByText("尚未检测", { exact: true })).toBeVisible();
  await expect(emptyCard.getByText("勾选账号后，在顶部开始检测", { exact: true })).toBeVisible();
  await expect(emptyCard.getByRole("button", { name: /放大查看/ })).toHaveCount(0);
  await cards.first().focus();
  await page.keyboard.press("Space");
  await expect(cards.first()).toBeChecked();
  await dialog.getByRole("combobox", { name: "检测模型" }).fill("shared-model");
  const settings = dialog.getByRole("group", { name: "动画检测设置", exact: true });
  expect(await settings.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  const filters = dialog.getByRole("group", { name: "动画账号筛选", exact: true });
  const operations = dialog.getByRole("group", { name: "动画检测操作", exact: true });
  const region = dialog.getByRole("region", { name: "动画账号卡片", exact: true });
  const pagination = dialog.getByRole("navigation", { name: "动画账号分页", exact: true });
  const filterBox = (await filters.boundingBox())!;
  const operationBox = (await operations.boundingBox())!;
  expect(filterBox.y + filterBox.height).toBeLessThanOrEqual(operationBox.y);
  const operationStart = operations.getByRole("button", { name: /开始检测/ });
  await expect(operationStart).toBeInViewport({ ratio: 1 });
  await expect(pagination).toBeInViewport({ ratio: 1 });
  const startBefore = (await operationStart.boundingBox())!;
  const footerBefore = (await pagination.boundingBox())!;
  await region.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
  });
  expect(await region.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  expect((await operationStart.boundingBox())!.y).toBe(startBefore.y);
  expect((await pagination.boundingBox())!.y).toBe(footerBefore.y);
  for (const card of await dialog.getByRole("article").all())
    await expect(card).toHaveCSS("height", "360px");
  await dialog.getByRole("button", { name: "转到下一页" }).click();
  await expect.poll(() => region.evaluate((element) => element.scrollTop)).toBe(0);
  await dialog.getByRole("button", { name: "转到上一页" }).click();
  await expect(
    dialog.getByRole("article", { name: "账号 批量账号 1", exact: true }).getByRole("img"),
  ).toBeVisible();
  await expect(
    dialog
      .getByRole("article", { name: "账号 批量账号 2", exact: true })
      .getByRole("button", { name: "重试 批量账号 2" }),
  ).toBeEnabled();

  await expect(dialog.getByRole("combobox", { name: "最近检测任务" })).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "仅看本次检测" })).toHaveCount(0);
  await page.screenshot({ path: test.info().outputPath("animation-selection-tabs.png") });
  await dialog.getByRole("button", { name: "转到下一页" }).click();
  await expect(cards).toHaveCount(12);
  await dialog.getByRole("checkbox", { name: /^检测 批量账号 13\b/ }).check();
  const search = dialog.getByRole("textbox", { name: "搜索动画检测账号" });
  await search.fill("批量账号 240");
  await expect(cards).toHaveCount(1);
  await expect(dialog.getByRole("checkbox", { name: /^检测 批量账号 240\b/ })).toBeVisible();
  await search.fill("");
  await expect(cards).toHaveCount(12);
  await expect(cards.first()).toBeChecked();
  await expect(dialog.getByRole("combobox", { name: "检测模型" })).toHaveValue("shared-model");
  const start = dialog.getByRole("button", { name: "开始检测（2 个账号）" });
  await expect(start).toBeInViewport({ ratio: 1 });
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await start.click();
  const confirmation = page.getByRole("dialog", { name: "确认动画检测范围" });
  await expect(confirmation).toContainText("ID 1）→ shared-model");
  await expect(confirmation).toContainText("ID 13）→ shared-model");
  await confirmation.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("tab", { name: "常规检测" }).click();
  await entry.click();
  await expect(dialog.getByRole("combobox", { name: "检测模型" })).toHaveValue("shared-model");
  await expect(dialog.getByRole("button", { name: "开始检测（2 个账号）" })).toBeEnabled();
  for (const [label, option] of [
    ["分组", "主组"],
    ["平台", "openai"],
    ["优先状态", "人工优先"],
  ]) {
    await dialog.getByRole("button", { name: label + "筛选" }).click();
    await page.getByRole("option", { name: option }).click();
    await page.keyboard.press("Escape");
  }
  await expect(cards).toHaveCount(1);
  await expect(cards.first()).toBeChecked();
  await search.fill("没有匹配账号");
  await expect(dialog.getByText("没有匹配的账号", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "开始检测（2 个账号）" })).toBeEnabled();
  await dialog.getByRole("button", { name: "重置筛选" }).click();
  await expect(cards).toHaveCount(12);
  await expect(dialog.getByLabel("结果展示时长（秒）")).toHaveCount(0);
});
