import { expect, test } from "@playwright/test";

const modelPrices = [{ model: "model-a", input_ratio: "0.5", completion_ratio: "4" }];
const references = [
  {
    model: "model-a",
    input_price: "0.000001",
    output_price: "0.000004",
    model_ratio: "0.5",
    completion_ratio: "4",
  },
];

test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "刷新回归测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi": {
        platforms: [
          {
            id: "primary",
            name: "测试平台",
            base_url: "https://newapi.example",
            user_id: "1",
            admin_key_configured: true,
          },
        ],
        local_groups: [{ id: "6", name: "标准", ratio: "1" }],
        bindings: [],
        sub2api_base_url: "https://sub2api.example",
      },
      "/api/newapi/platforms/primary/refresh": {
        groups: [{ id: "default", name: "默认", ratio: "1" }],
        models: modelPrices,
        unset_models: [],
        references: [],
        tool_prices: [],
        differences: [],
      },
      "/api/auth-recovery/config": {
        vault_entries: [
          {
            entry: "运营账号",
            hosts: ["sub2api.example"],
            has_username: true,
            has_password: true,
            username_is_email: true,
            header_names: [],
          },
        ],
      },
      "/api/newapi/platforms/primary/channel-key": {
        key_id: "key-7",
        name: "标准",
        group_id: "6",
        endpoints: [{ name: "API", base_url: "https://api.example", default: true }],
      },
      "/api/accounts": [
        {
          id: "41",
          name: "测试账号",
          groups: [],
          platform: "openai",
          account_type: "apikey",
          health: "healthy",
          schedulable: true,
        },
      ],
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: ["gpt-5.6-sol"] },
      "/api/model-checks/account-statuses": [],
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("比较后的批量预览与远程表格复用缓存，主动刷新保留分页和模型行", async ({ page }) => {
  let priceReads = 0;
  await page.route("**/management-model-prices", async (route) => {
    priceReads += 1;
    await route.fulfill({
      json: { models: references, expires_at: new Date(Date.now() + 86400000).toISOString() },
    });
  });
  let releaseRefresh = (): void => {};
  const refreshed = new Promise<void>((resolve) => {
    releaseRefresh = resolve;
  });
  await page.route("**/management-model-prices/refresh", async (route) => {
    await refreshed;
    await route.fulfill({
      json: { models: references, expires_at: new Date(Date.now() + 86400000).toISOString() },
    });
  });
  await page.goto("/newapi/prices");
  await page.getByRole("button", { name: "比较模型价格", exact: true }).click();
  const status = page.getByText("一致", { exact: true });
  await expect(status).toBeVisible();
  const originalStatus = await status.elementHandle();
  await page.getByRole("checkbox", { name: "选择本页模型", exact: true }).check();
  await page.getByRole("button", { name: "批量同步（1）" }).click();
  const dialog = page.getByRole("dialog", { name: "批量同步模型价格" });
  await expect(dialog.getByRole("button", { name: "确认同步 1 个模型" })).toBeEnabled();
  expect(priceReads).toBe(1);
  expect(
    await originalStatus!.evaluate(
      (element) => element.isConnected && element.textContent === "一致",
    ),
  ).toBe(true);
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("tab", { name: "远程模型价格", exact: true }).click();
  const row = page.getByRole("row").filter({ has: page.getByText("model-a", { exact: true }) });
  const originalRow = await row.elementHandle();
  const pagination = page.getByRole("button", { name: "转到下一页" });
  const originalPagination = await pagination.elementHandle();
  await page.getByRole("button", { name: "强制刷新参考价格" }).click();
  await expect(page.getByRole("button", { name: "强制刷新参考价格" })).toBeDisabled();
  await expect(pagination).toBeVisible();
  expect(await originalPagination!.evaluate((element) => element.isConnected)).toBe(true);
  expect(await originalRow!.evaluate((element) => element.isConnected)).toBe(true);
  releaseRefresh();
  await expect(page.getByRole("button", { name: "强制刷新参考价格" })).toBeEnabled();
  expect(priceReads).toBe(1);
});

test("重新获取渠道模型保留已有选择和背景数量，失败后禁止确认旧目录", async ({ page }) => {
  let reads = 0;
  let releaseRefresh = (): void => {};
  const refreshed = new Promise<void>((resolve) => {
    releaseRefresh = resolve;
  });
  await page.route("**/channel-models", async (route) => {
    reads += 1;
    if (reads === 1) {
      await route.fulfill({ json: { models: ["model-a"] } });
      return;
    }
    await refreshed;
    await route.fulfill({ status: 503, json: { detail: "上游连接失败" } });
  });
  await page.goto("/newapi/channels");
  await page.getByRole("combobox", { name: "Sub2API 分组" }).click();
  await page.getByRole("option", { name: "标准", exact: true }).click();
  await page.getByRole("button", { name: "创建密钥", exact: true }).click();
  await page.getByRole("button", { name: "从上游获取" }).click();
  const dialog = page.getByRole("dialog", { name: "选择上游模型" });
  await expect(dialog.getByRole("button", { name: "确认模型" })).toBeEnabled();
  await dialog.getByRole("button", { name: "确认模型" }).click();
  await expect(page.locator("#root").getByText("已选择 1 个模型")).toBeVisible();
  await page.getByRole("button", { name: "从上游获取" }).click();
  const selectedModel = dialog.getByRole("checkbox", { name: /model-a/ });
  await expect(selectedModel).toBeChecked();
  await expect(selectedModel).toBeDisabled();
  await expect(dialog.getByRole("status", { name: "正在从上游获取模型" })).toHaveCount(0);
  await expect(page.getByText("尚未选择模型")).toHaveCount(0);
  releaseRefresh();
  await expect(page.locator("[data-sonner-toast]")).toContainText("上游连接失败");
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await expect(selectedModel).toBeChecked();
  await expect(dialog.getByRole("button", { name: "确认模型" })).toBeDisabled();
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.getByRole("button", { name: "添加渠道", exact: true })).toBeDisabled();
});

test("模型检测后台刷新保持共同模型列表及选择，完成后恢复检测按钮", async ({ page }) => {
  let reads = 0;
  let releaseRefresh = (): void => {};
  const refreshed = new Promise<void>((resolve) => {
    releaseRefresh = resolve;
  });
  await page.route("**/accounts/41/models", async (route) => {
    reads += 1;
    if (reads > 1) await refreshed;
    await route.fulfill({ json: { models: ["gpt-5.6-sol"] } });
  });
  await page.goto("/model-check");
  await page.getByRole("button", { name: "全选账号" }).click();
  const model = page.getByRole("checkbox", { name: /gpt-5.6-sol/ });
  await model.check();
  const list = page.getByRole("list", { name: "可检测模型" });
  const originalList = await list.elementHandle();
  await page.getByRole("button", { name: "刷新模型" }).click();
  await expect(page.getByRole("button", { name: "刷新模型" })).toBeDisabled();
  expect(await originalList!.evaluate((element) => element.isConnected)).toBe(true);
  await expect(model).toBeChecked();
  await expect(page.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  releaseRefresh();
  await expect(page.getByRole("button", { name: /开始检测/ })).toBeEnabled();
  await expect(model).toBeChecked();
});

test("Key 清理重新扫描不清空已显示预览，扫描失败后禁止删除", async ({ page }) => {
  const upstream = {
    upstream_id: "1",
    host: "upstream.example",
    name: "测试上游",
    base_url: "https://upstream.example",
    account_base_url: "https://upstream.example",
    upstream_type: "sub2api",
    auth_mode: "access_token",
    recharge_rate: "1",
    raw_balance: "10",
    balance: "10",
    headers: {},
    header_names: [],
    cookie_names: [],
    groups: [],
    has_access_token: true,
    has_refresh_token: false,
    has_admin_key: false,
    has_user_id: false,
  };
  await page.route("**/api/upstreams/upstream.example/configuration", (route) =>
    route.fulfill({ json: upstream }),
  );
  await page.route("**/api/config", (route) =>
    route.fulfill({ json: { account_default_concurrency: 10, account_default_priority: 1 } }),
  );
  await page.route("**/api/groups", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/upstreams", (route) => route.fulfill({ json: { hosts: [] } }));
  await page.route("**/api/onboarding/prepare", (route) =>
    route.fulfill({ json: { upstream, candidates: [] } }),
  );
  let scans = 0;
  let releaseScan = (): void => {};
  const scanned = new Promise<void>((resolve) => {
    releaseScan = resolve;
  });
  await page.route("**/api/onboarding/keys/cleanup-preview", async (route) => {
    scans += 1;
    if (scans === 1) {
      await route.fulfill({
        json: {
          host: "upstream.example",
          keys: [{ key_id: "17", name: "unused-key", group_id: "6", status: "active" }],
        },
      });
    } else {
      await scanned;
      await route.fulfill({ status: 503, json: { detail: "扫描连接失败" } });
    }
  });
  await page.goto("/onboarding?host=upstream.example");
  await page.getByRole("button", { name: "清理无用 Key" }).click();
  const dialog = page.getByRole("dialog", { name: "清理无绑定上游 Key" });
  await expect(dialog.getByText("unused-key", { exact: true })).toBeVisible();
  const originalTable = await dialog.getByRole("table").elementHandle();
  const width = await dialog.evaluate((element) => element.clientWidth);
  await dialog.getByRole("button", { name: "刷新扫描结果" }).click();
  await expect(dialog.getByRole("button", { name: "刷新扫描结果" })).toBeDisabled();
  expect(await originalTable!.evaluate((element) => element.isConnected)).toBe(true);
  expect(await dialog.evaluate((element) => element.clientWidth)).toBe(width);
  await expect(dialog.getByText("正在扫描上游 Key 与绑定关系")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "确认删除 1 个 Key" })).toBeDisabled();
  releaseScan();
  await expect(page.locator("[data-sonner-toast]")).toContainText("扫描连接失败");
  await expect(dialog.getByRole("alert")).toHaveCount(0);
  await expect(dialog.getByText("unused-key", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "确认删除 1 个 Key" })).toBeDisabled();
});
