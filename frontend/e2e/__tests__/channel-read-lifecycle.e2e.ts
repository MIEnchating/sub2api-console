import { expect, test, type Page, type Request, type Route } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

const channels = Array.from({ length: 5 }, (_, index) => ({
  id: String(index + 1),
  name: `渠道 ${index + 1}`,
  type: 59,
  status: 1,
  models: ["old"],
  groups: ["default"],
  version: "a".repeat(64),
}));

async function isolate(page: Page): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "渠道读取测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/channels": { items: channels, total: 5 },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path === "/api/dictionaries") await route.fulfill({ json: { items: [] } });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "未配置隔离接口" } });
  });
}

async function openDiscovery(page: Page): Promise<void> {
  await page.goto("/newapi/channels");
  await page.getByRole("checkbox", { name: "全选当前筛选渠道" }).check();
  await page.getByRole("button", { name: "批量上架模型", exact: true }).click();
  await page.getByLabel("上架模型名称").fill("manual-model");
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
}

async function holdModelReads(page: Page): Promise<Map<Request, Route>> {
  const active = new Map<Request, Route>();
  page.on("requestfailed", (request) => active.delete(request));
  await page.route("**/channels/*/models/available?*", async (route) => {
    // StrictMode may cancel and remount a query. Count only requests still live.
    if (!route.request().failure()) active.set(route.request(), route);
  });
  return active;
}

test.beforeEach(async ({ page, colorScheme, viewport }) => {
  if (viewport!.width < 640) await page.setViewportSize({ width: 320, height: 568 });
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await isolate(page);
});

test("批量模型分批读取后才允许键盘确认，保留手输内容且不直接提交写入", async ({ page }) => {
  const active = await holdModelReads(page);
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET") writes.push(request.url());
  });
  await openDiscovery(page);
  const picker = page.getByRole("dialog", { name: "选择上游模型" });
  await expect.poll(() => active.size).toBe(4);
  await expect(picker.getByRole("button", { name: "确认模型" })).toBeDisabled();
  await Promise.all(
    [...active.entries()].map(async ([request, route]) => {
      active.delete(request);
      await route.fulfill({ json: { models: ["old", "common", "batch-only"] } });
    }),
  );
  await expect
    .poll(() => [...active.keys()].map((request) => new URL(request.url()).pathname))
    .toEqual(["/api/newapi/platforms/layout/channels/5/models/available"]);
  await expect(picker.getByRole("checkbox")).toHaveCount(0);
  await [...active.values()][0].fulfill({ json: { models: ["old", "common"] } });
  const choice = picker.getByRole("checkbox", { name: /common/ });
  await expect(picker.getByRole("checkbox")).toHaveCount(1);
  await choice.focus();
  await page.keyboard.press("Space");
  await expect(choice).toBeChecked();
  await expect(picker.getByRole("button", { name: "确认模型" })).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("batch-models-ready.png") });
  await picker.getByRole("button", { name: "确认模型" }).click();
  await expect(page.getByLabel("上架模型名称")).toHaveValue("manual-model\ncommon");
  expect(writes).toEqual([]);
});

test("取消批量模型读取会取消四个在途请求，重开读取新结果", async ({ page }) => {
  const active = await holdModelReads(page);
  await openDiscovery(page);
  await expect.poll(() => active.size).toBe(4);
  const cancellations = [...active.keys()].map((request) =>
    page.waitForEvent("requestfailed", { predicate: (failed) => failed === request }),
  );
  const picker = page.getByRole("dialog", { name: "选择上游模型" });
  await picker.getByRole("button", { name: "取消", exact: true }).click();
  await Promise.all(cancellations);
  expect(active.size).toBe(0);
  await expect(picker).toHaveCount(0);
  await expect(page.getByLabel("上架模型名称")).toHaveValue("manual-model");
  await page.route("**/channels/*/models/available?*", (route) =>
    route.fulfill({ json: { models: ["fresh"] } }),
  );
  await page.getByRole("button", { name: "获取模型", exact: true }).click();
  await expect(picker.getByRole("checkbox", { name: /fresh/ })).toBeVisible();
  expect(active.size).toBe(0);
});

test("离开渠道页会取消列表和渠道分组的在途请求", async ({ page }) => {
  const paths = [
    "/api/newapi/platforms/layout/channels",
    "/api/newapi/platforms/layout/channel-groups",
  ];
  const held = new Set<string>();
  const aborted = new Set<string>();
  page.on("requestfailed", (request) => {
    const path = new URL(request.url()).pathname;
    if (paths.includes(path)) aborted.add(path);
  });
  await page.route("**/api/newapi/platforms/layout/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (paths.includes(path)) {
      held.add(path);
      return;
    }
    await route.fallback();
  });
  await page.goto("/newapi/channels");
  await expect.poll(() => held.size).toBe(2);
  await expect(page.getByRole("status", { name: "正在读取现有渠道" })).toBeVisible();
  aborted.clear();
  if (page.viewportSize()!.width < 640)
    await page.getByRole("button", { name: "切换侧边栏" }).click();
  await page.getByRole("link", { name: "账号模型", exact: true }).click();
  await expect(page).toHaveURL(/account-model-automation/);
  await expect.poll(() => aborted.size).toBe(2);
});
