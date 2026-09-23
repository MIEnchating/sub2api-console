import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test.beforeEach(async ({ page }, info) => {
  await page.addInitScript(
    (theme: string) => {
      localStorage.setItem("sub2api-console-theme", theme);
    },
    info.project.name === "desktop-light" ? "light" : "dark",
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "工作台验收" },
      "/api/dictionaries": { items: [] },
      "/api/account-workbench/templates": { revision: 0, preferred_id: "", items: [] },
      "/api/account-workbench/runs": [],
      "/api/account-workbench/accounts": [
        {
          id: "41",
          name: "测试账号",
          email: "owner@example.test",
          status: "active",
          schedulable: true,
          plan: "plus",
          groups: [{ id: "7", name: "默认分组" }],
          proxy_name: "",
          concurrency: "1",
          load_factor: "1",
          rate_multiplier: "1",
          model_mapping: {},
          fingerprint: "off",
        },
      ],
      "/api/account-workbench/maintenance": {
        revision: 0,
        enabled: false,
        interval_minutes: 5,
        cooldown_minutes: 10,
        check_after_repair: true,
        group_ids: [],
        running: false,
        task_id: "",
        last_check_at: null,
        next_check_at: null,
        message: "",
        results: [],
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
});

test("工作台五个功能页键盘切换同步选中状态且窄屏不撑宽页面", async ({ page }) => {
  await page.goto("/account-workbench");
  await expect(page.getByRole("heading", { name: "账号工作台", exact: true })).toHaveCount(0);
  const tabs = page.getByRole("tablist", { name: "账号工作台功能" });
  await expect(tabs.getByRole("tab")).toHaveCount(5);
  const names = ["导入账号", "账号列表", "配置模板", "处理记录", "自动维护"];
  for (const name of names) {
    await tabs.getByRole("tab", { name, exact: true }).click();
    await expect(page.getByRole("heading", { name, exact: true })).toHaveCount(0);
    await expect(tabs.getByRole("tab", { name, exact: true })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(page.locator('[data-slot="skeleton"]')).toHaveCount(0);
    const panel = page.getByRole("tabpanel");
    await expect(panel).toBeVisible();
    expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await page.screenshot({ path: test.info().outputPath(`${name}.png`), fullPage: true });
    if (name === "自动维护") {
      const save = page.getByRole("button", { name: "预览并保存" });
      await save.scrollIntoViewIfNeeded();
      await expect(save).toBeInViewport();
    }
  }
  await tabs.getByRole("tab", { name: "自动维护" }).focus();
  await page.keyboard.press("Home");
  await expect(tabs.getByRole("tab", { name: "导入账号" })).toBeFocused();
  await expect(page.getByRole("textbox", { name: "账号资料", exact: true })).toBeVisible();
  await page.keyboard.press("ArrowRight");
  await expect(tabs.getByRole("tab", { name: "账号列表" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByRole("searchbox", { name: "搜索账号" })).toBeVisible();
});

test("统一上传入口能选择 TXT 文件并把内容填入账号输入框", async ({ page }) => {
  await page.goto("/account-workbench");
  await page.locator('input[type="file"]').setInputFiles({
    name: "accounts.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("rt_isolated\nowner@example.test----test-password"),
  });
  await expect(page.getByRole("textbox", { name: "账号资料", exact: true })).toHaveValue(
    "rt_isolated\nowner@example.test----test-password",
  );
  await expect(page.getByText("accounts.txt", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "解析并预览" })).toBeEnabled();
  const upload = await page.getByRole("group", { name: "文件上传" }).boundingBox();
  const inputCard = await page.getByRole("region", { name: "账号资料", exact: true }).boundingBox();
  expect(upload!.y + upload!.height).toBeLessThan(inputCard!.y + inputCard!.height);
});

test("导入页桌面分栏，窄屏滚动设置时解析按钮始终可见", async ({ page }, info) => {
  await page.goto("/account-workbench");
  const input = page.getByRole("textbox", { name: "账号资料", exact: true });
  const action = page.getByRole("combobox", { name: "处理方式", exact: true });
  const submit = page.getByRole("button", { name: "解析并预览" });
  await expect(input).toBeVisible();
  await expect(submit).toBeInViewport();
  if (info.project.name === "desktop-light") {
    const inputBox = await input.boundingBox();
    const actionBox = await action.boundingBox();
    expect(actionBox!.x).toBeGreaterThan(inputBox!.x + inputBox!.width);
    expect(actionBox!.y).toBeLessThan(inputBox!.y + 80);
  } else {
    const inputCard = await page
      .getByRole("region", { name: "账号资料", exact: true })
      .boundingBox();
    const settings = await page
      .getByRole("region", { name: "处理设置", exact: true })
      .boundingBox();
    expect(settings!.y).toBeGreaterThan(inputCard!.y + inputCard!.height);
    await input.click();
    await expect(input).toBeFocused();
  }
  const proxy = page.getByRole("checkbox", { name: "使用登录 / 检测代理" });
  await proxy.scrollIntoViewIfNeeded();
  await proxy.check();
  await expect(page.getByLabel("登录代理地址")).toBeVisible();
  await expect(submit).toBeInViewport();
  await input.fill("rt_" + "a".repeat(3000));
  await expect(submit).toBeInViewport();
  expect(
    await page
      .getByRole("tabpanel")
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
});

test("低高度窗口展开检测设置后仍能操作，切换输出方式隐藏导入专属设置", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 640 });
  await page.goto("/account-workbench");
  await page.getByText("检测设置", { exact: true }).click();
  const model = page.getByRole("textbox", { name: "检测模型" });
  await model.scrollIntoViewIfNeeded();
  await expect(model).toBeInViewport();
  await expect(page.getByRole("button", { name: "解析并预览" })).toBeInViewport();
  await page.getByRole("combobox", { name: "处理方式", exact: true }).click();
  await page.getByRole("option", { name: "独立 JSON 输出" }).click();
  await expect(page.getByRole("combobox", { name: "配置模板" })).toHaveCount(0);
  await expect(model).toHaveCount(0);
  await expect(page.getByRole("button", { name: "解析并预览" })).toBeInViewport();
});

test("账号筛选显示名称，维护设置和结果在桌面分栏、手机顺序排列", async ({ page }, info) => {
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "账号列表", exact: true }).click();
  const status = page.getByRole("combobox", { name: "筛选账号状态" });
  const group = page.getByRole("combobox", { name: "筛选账号分组" });
  await expect(status).toContainText("全部状态");
  await expect(group).toContainText("全部分组");
  await group.click();
  await page.getByRole("option", { name: "默认分组", exact: true }).click();
  await expect(group).toContainText("默认分组");
  await page.getByRole("tab", { name: "自动维护", exact: true }).click();
  const settings = page.getByRole("form", { name: "维护设置" });
  const results = page.getByRole("region", { name: "维护结果" });
  await expect(settings).toBeVisible();
  await expect(results).toBeVisible();
  const settingsBox = await settings.boundingBox();
  const resultsBox = await results.boundingBox();
  if (info.project.name === "desktop-light") {
    expect(resultsBox!.x).toBeGreaterThan(settingsBox!.x + settingsBox!.width);
    expect(resultsBox!.y).toBe(settingsBox!.y);
  } else {
    expect(resultsBox!.y).toBeGreaterThan(settingsBox!.y + settingsBox!.height);
  }
});

test("多模板自适应排列，长名称和处理记录展开不会撑宽页面", async ({ page }, info) => {
  const template = {
    id: "template-one",
    name: "team-template-".repeat(15),
    source_id: "41",
    source_name: "来源账号",
    source_version: "1",
    synced_at: "2026-09-17T00:00:00Z",
    revision: 1,
    config: {
      concurrency: 1,
      priority: 50,
      rate_multiplier: "1",
      load_factor: null,
      proxy_id: null,
      group_ids: [],
      auto_pause_on_expired: true,
      expires_at: null,
      notes: "",
      credential_extras: {},
      extra: {},
    },
    summary: { proxy_name: "", groups: [] },
  };
  await page.route("**/api/account-workbench/templates", (route) =>
    route.fulfill({
      json: {
        revision: 1,
        preferred_id: template.id,
        items: [template, { ...template, id: "template-two", name: "备用配置" }],
      },
    }),
  );
  await page.route("**/api/account-workbench/runs", (route) =>
    route.fulfill({
      json: [
        {
          id: "run-one",
          revision: 1,
          status: "needs_attention",
          action: "import",
          task_id: "task-one",
          created_at: "2026-09-17T00:00:00Z",
          updated_at: "2026-09-17T00:00:00Z",
          expires_at: "2099-01-01T00:00:00Z",
          duplicate_count: 0,
          items: [
            {
              id: "item-one",
              index: 0,
              kind: "codex_json",
              name: "",
              email: "very-long-account-name".repeat(8) + "@example.test",
              plan: "",
              identity_source: "official_signature",
              status: "review",
              message: "需要复核",
              template_name: "团队配置",
              account_id: "41",
              check: { verdict: "MISMATCH" },
            },
          ],
        },
      ],
    }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "配置模板", exact: true }).click();
  const cards = page.getByRole("article");
  await expect(cards).toHaveCount(2);
  const first = await cards.nth(0).boundingBox();
  const second = await cards.nth(1).boundingBox();
  if (info.project.name === "desktop-light")
    expect(second!.x).toBeGreaterThan(first!.x + first!.width);
  else expect(second!.y).toBeGreaterThan(first!.y + first!.height);
  const panel = page.getByRole("tabpanel");
  expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("模板列表.png"), fullPage: true });
  await page.getByRole("tab", { name: "处理记录", exact: true }).click();
  await expect(page.getByText("检测不匹配", { exact: true })).toBeVisible();
  const expand = page.getByRole("button", { name: "账号导入 · 1 项", exact: true });
  await expand.click();
  await expect(expand).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByRole("table", { name: "账号处理结果" })).toHaveCount(0);
  await expand.click();
  await expect(page.getByRole("table", { name: "账号处理结果" })).toBeVisible();
  expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("处理记录.png"), fullPage: true });
});

test("没有线上账号时手动创建模板，保存后可编辑并在导入中选择", async ({ page }) => {
  const writes: Array<Record<string, unknown>> = [];
  let items: Array<Record<string, unknown>> = [];
  await page.route("**/api/account-workbench/accounts", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/account-workbench/templates", async (route) => {
    if (route.request().method() === "POST") {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      writes.push(body);
      items = [
        {
          ...body,
          id: "manual-one",
          source_id: "",
          source_name: "",
          source_version: "",
          synced_at: "2026-09-18T00:00:00Z",
          revision: writes.length,
          summary: { groups: [], proxy_name: "" },
        },
      ];
    }
    await route.fulfill({
      json: { revision: writes.length, preferred_id: items.length ? "manual-one" : "", items },
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "配置模板", exact: true }).click();
  await page.getByRole("button", { name: "创建模板", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "手动创建模板" })).toBeVisible();
  await dialog.getByRole("textbox", { name: "模板名称", exact: true }).fill("手动测试模板");
  await dialog.getByRole("spinbutton", { name: "并发数" }).fill("0");
  await dialog.getByRole("textbox", { name: "计费倍率" }).fill("0.1234567890123456789");
  await dialog.getByRole("combobox", { name: "订阅档位" }).click();
  await page.getByRole("option", { name: "Plus", exact: true }).click();
  await dialog.getByRole("textbox", { name: "备注", exact: true }).fill("手动填写");
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const save = dialog.getByRole("button", { name: "保存模板" });
  await save.scrollIntoViewIfNeeded();
  await expect(save).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("手动模板.png"), fullPage: true });
  await save.click();
  await expect(dialog).toHaveCount(0);
  expect(writes[0]).toMatchObject({
    name: "手动测试模板",
    config: {
      concurrency: 0,
      rate_multiplier: "0.1234567890123456789",
      credential_extras: { plan_type: "plus" },
    },
  });
  expect(writes[0]).not.toHaveProperty("source_id");
  await page.getByRole("button", { name: "编辑配置" }).click();
  await expect(dialog.getByRole("spinbutton", { name: "并发数" })).toHaveValue("0");
  await expect(dialog.getByRole("combobox", { name: "订阅档位" })).toContainText("Plus");
  await dialog.getByRole("textbox", { name: "模板名称", exact: true }).fill("更新手动模板");
  await dialog.getByRole("button", { name: "保存模板" }).click();
  await expect(dialog).toHaveCount(0);
  expect(writes[1]).toMatchObject({ id: "manual-one", revision: 1, name: "更新手动模板" });
  await page.getByRole("tab", { name: "导入账号", exact: true }).click();
  await page.getByRole("combobox", { name: "配置模板", exact: true }).click();
  await page.getByRole("option", { name: "更新手动模板", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "配置模板", exact: true })).toContainText(
    "更新手动模板",
  );
});

test("所有工作台标签在窄屏完整可见，键盘末项切换与面板选中状态一致", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 664 });
  await page.goto("/account-workbench");
  const tabs = page.getByRole("tablist", { name: "账号工作台功能" });
  for (const tab of await tabs.getByRole("tab").all()) {
    const box = await tab.boundingBox();
    const container = await tabs.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(container!.x);
    expect(box!.x + box!.width).toBeLessThanOrEqual(container!.x + container!.width);
  }
  await tabs.getByRole("tab", { name: "导入账号" }).focus();
  await page.keyboard.press("End");
  await expect(tabs.getByRole("tab", { name: "自动维护" })).toBeFocused();
  await expect(page.getByRole("tabpanel", { name: "自动维护" })).toBeVisible();
});

test("账号表格横向滚动时查看入口保持可见，详情中的长映射独立显示", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 664 });
  const model = "long-model-name-".repeat(20);
  await page.route("**/api/account-workbench/accounts", (route) =>
    route.fulfill({
      json: [
        {
          id: "41",
          name: "测试账号",
          email: "owner@example.test",
          status: "active",
          schedulable: true,
          plan: "plus",
          groups: [],
          proxy_name: "",
          concurrency: "1",
          load_factor: "1",
          rate_multiplier: "1",
          model_mapping: { [model]: "gpt-target" },
          fingerprint: "off",
        },
      ],
    }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "账号列表", exact: true }).click();
  const action = page.getByRole("button", { name: "查看 测试账号 配置" });
  await expect(action).toBeInViewport();
  const table = page.getByRole("table", { name: "账号列表", exact: true });
  await table.locator("..").evaluate((element) => {
    element.scrollLeft = element.scrollWidth;
  });
  await expect(action).toBeInViewport();
  await action.click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("region", { name: "模型映射" })).toBeVisible();
  await expect(dialog.getByRole("cell", { name: model, exact: true })).toBeVisible();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "关闭", exact: true }).last()).toBeInViewport();
});

test("维护结果包含长邮箱时仅表格横向滚动，运行时间和停止入口清晰可见", async ({ page }) => {
  const email = "long-account-name-".repeat(12) + "@example.test";
  await page.route("**/api/account-workbench/maintenance", (route) =>
    route.fulfill({
      json: {
        revision: 2,
        enabled: true,
        interval_minutes: 5,
        cooldown_minutes: 10,
        check_after_repair: true,
        group_ids: [],
        running: true,
        task_id: "maintenance-layout",
        last_check_at: "2026-09-20T10:00:00Z",
        next_check_at: "2026-09-20T10:05:00Z",
        message: "",
        results: [
          {
            account_id: "41",
            email,
            action: "refresh",
            status: "cooldown",
            reason: "rate_limited",
          },
        ],
      },
    }),
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "自动维护", exact: true }).click();
  const results = page.getByRole("region", { name: "维护结果" });
  await expect(results.getByRole("status")).toContainText("正在维护账号");
  await expect(results.getByText("上次检查", { exact: true })).toBeVisible();
  await expect(results.getByText("下次检查", { exact: true })).toBeVisible();
  await expect(results.getByRole("cell", { name: email })).toBeVisible();
  expect(await results.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(
    await page
      .getByRole("tabpanel")
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  const stop = page.getByRole("button", { name: "停止维护" });
  await stop.scrollIntoViewIfNeeded();
  await expect(stop).toBeInViewport();
  await expect(stop).toBeEnabled();
  await expect(page.getByRole("button", { name: "预览并保存" })).toBeDisabled();
  await page.screenshot({ path: test.info().outputPath("维护运行结果.png"), fullPage: true });
});
