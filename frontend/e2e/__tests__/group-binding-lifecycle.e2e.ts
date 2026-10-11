import { expect, test } from "@playwright/test";
import { pageFixtures, workspace } from "./fixtures/page-shell";

const groups = [
  { id: "vip", name: "VIP", ratio: "1" },
  { id: "premium", name: "高级", ratio: "1" },
];
const binding = {
  platform_id: "layout",
  newapi_group_id: "vip",
  newapi_group_name: "VIP",
  sub2api_group_id: "6",
  sync_ratio: false,
};

test.beforeEach(async ({ page, colorScheme, viewport }) => {
  if (viewport!.width < 640) await page.setViewportSize({ width: 320, height: 568 });
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "绑定回归" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": { items: [{ value: "6", name: "标准", enabled: true }] },
      "/api/newapi": { ...workspace, bindings: [binding] },
      "/api/newapi/platforms/layout/groups": { groups },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (route.request().method() === "GET" && path in fixtures)
      await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离接口未配置" } });
  });
});

test("同组新绑定沿用手动倍率，筛选后保存仍提交完整绑定并显示失败重试", async ({ page }) => {
  const writes: unknown[] = [];
  const snapshots: string[] = [];
  page.on("request", (request) => {
    if (request.url().endsWith("/refresh")) snapshots.push(request.url());
  });
  await page.route("**/group-bindings", async (route) => {
    writes.push(route.request().postDataJSON());
    await route.fulfill({ status: 409, json: { detail: "分组目录已变化，请刷新核对后再保存" } });
  });
  await page.goto("/newapi/groups");
  await page.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" }).fill("0.75");
  const select = page.getByRole("combobox", { name: "高级 的 Sub2API 分组" });
  await select.focus();
  await page.keyboard.press("ArrowDown");
  await page.getByRole("option", { name: "标准 · 1", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "高级 的 Sub2API 管理平台倍率" })).toHaveValue(
    "0.75",
  );
  await page.getByRole("textbox", { name: "搜索 New API 分组或 ID" }).fill("vip");
  await page.getByRole("button", { name: "保存绑定与倍率" }).click();
  await expect(
    page.getByText("分组目录已变化，请刷新核对后再保存", { exact: false }),
  ).toBeVisible();
  expect(writes).toEqual([
    {
      bindings: groups.map((group) => ({
        newapi_group_id: group.id,
        newapi_group_name: group.name,
        sub2api_group_id: "6",
        sub2api_ratio: "0.75",
        sync_ratio: false,
      })),
    },
  ]);
  await expect(page.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" })).toHaveValue(
    "0.75",
  );
  await expect(page.getByRole("button", { name: "保存绑定与倍率" })).toBeEnabled();
  expect(snapshots).toEqual([]);
  await page.screenshot({ path: test.info().outputPath("binding-draft-retained.png") });
});

test("刷新本地倍率时保留当前表格且等待读取结束，离页取消远端读取", async ({ page }) => {
  await page.goto("/newapi/groups");
  const ratio = page.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" });
  await expect(ratio).toHaveValue("1");
  const waiting = page.waitForRequest((request) =>
    request.url().endsWith("/api/newapi?platform_id=layout"),
  );
  let release!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/newapi?platform_id=layout", async (route) => {
    await gate;
    await route.fulfill({
      json: {
        ...workspace,
        bindings: [binding],
        local_groups: [{ id: "6", name: "标准", ratio: "0.8" }],
      },
    });
  });
  await page.getByRole("button", { name: "刷新 New API 数据" }).click();
  await waiting;
  await expect(ratio).toHaveValue("1");
  await expect(page.getByRole("button", { name: "刷新 New API 数据" })).toBeDisabled();
  release();
  await expect(ratio).toHaveValue("0.8");
  await page.route("**/api/newapi/platforms/layout/groups", () => {});
  const pending = page.waitForRequest((request) =>
    request.url().endsWith("/platforms/layout/groups"),
  );
  await page.getByRole("button", { name: "刷新 New API 数据" }).click();
  const request = await pending;
  const cancelled = page.waitForEvent("requestfailed", {
    predicate: (failed) => failed === request,
  });
  if (page.viewportSize()!.width < 640)
    await page.getByRole("button", { name: "切换侧边栏" }).click();
  await page.getByRole("link", { name: "账号模型", exact: true }).click();
  await cancelled;
  await expect(page).toHaveURL(/account-model-automation/);
});

test("仅修改同步开关后刷新采用新倍率，保存成功后读回同步状态", async ({ page }) => {
  let saved = false;
  await page.goto("/newapi/groups");
  await page.getByRole("switch", { name: "VIP 倍率同步", exact: true }).click();
  await page.route("**/api/newapi?platform_id=layout", (route) =>
    route.fulfill({
      json: {
        ...workspace,
        bindings: [{ ...binding, sync_ratio: saved }],
        local_groups: [{ id: "6", name: "标准", ratio: "0.8" }],
      },
    }),
  );
  await page.getByRole("button", { name: "刷新 New API 数据" }).click();
  await expect(page.getByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" })).toHaveValue(
    "0.8",
  );
  await expect(page.getByRole("switch", { name: "VIP 倍率同步", exact: true })).toBeChecked();
  await page.route("**/group-bindings", async (route) => {
    expect(route.request().postDataJSON()).toEqual({
      bindings: [
        {
          newapi_group_id: "vip",
          newapi_group_name: "VIP",
          sub2api_group_id: "6",
          sub2api_ratio: "0.8",
          sync_ratio: true,
        },
      ],
    });
    saved = true;
    await route.fulfill({ json: [{ ...binding, sync_ratio: true }] });
  });
  await page.getByRole("button", { name: "保存绑定与倍率" }).click();
  await expect(page.getByText("分组绑定已保存", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "保存绑定与倍率" })).toBeEnabled();
  expect(saved).toBe(true);
});
