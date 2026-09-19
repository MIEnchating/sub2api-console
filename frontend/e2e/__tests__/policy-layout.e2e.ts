import { expect, test } from "@playwright/test";

import { policy } from "./fixtures/settings";

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
    } else if (path.startsWith("/api/dictionaries")) {
      await route.fulfill({ json: { items: [] } });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

for (const category of ["调度与写入", "健康与处置", "巡检与采样", "守护范围"]) {
  test(`${category} 在当前屏幕完整排列，滚动后分类与保存仍可操作`, async ({ page }) => {
    await page.goto("/policy");
    await page.getByRole("combobox", { name: "全局默认策略", exact: true }).waitFor();
    await page.getByRole("tab", { name: category, exact: true }).click();
    const panel = page.getByRole("tabpanel", { name: category, exact: true });
    await expect(panel).toBeVisible();
    expect(await panel.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    const overflowed = await panel
      .locator('[data-slot="policy-fields"]')
      .evaluateAll(
        (fields) => fields.filter((field) => field.scrollWidth > field.clientWidth).length,
      );
    expect(overflowed).toBe(0);
    const content = page.locator('[data-slot="page-content"]');
    await content.evaluate((element) => element.scrollTo(0, element.scrollHeight));
    await expect(page.getByRole("tablist", { name: "策略分类" })).toBeInViewport({ ratio: 1 });
    await expect(page.getByRole("button", { name: "保存策略", exact: true })).toBeInViewport({
      ratio: 1,
    });
    expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await content.evaluate((element) => element.scrollTo(0, 0));
    await page.screenshot({ path: test.info().outputPath("policy-layout.png") });
  });
}

test("卡片字段按实际宽度排列，桌面半宽卡片不挤成三列", async ({ page, viewport }) => {
  await page.goto("/policy");
  await page.getByRole("tab", { name: "健康与处置", exact: true }).click();
  const formulas = page.getByRole("region", { name: "健康分公式", exact: true });
  await expect(formulas).toBeVisible();
  const columns = await formulas
    .locator('[data-slot="policy-fields"]')
    .evaluate((element) => getComputedStyle(element).gridTemplateColumns.split(" ").length);
  expect(columns).toBe(viewport!.width >= 1280 ? 2 : 1);
  const keywords = page.getByRole("textbox", { name: "致命错误关键字（每行一个）" });
  const statusCodes = page.getByRole("textbox", { name: "网关错误状态码" });
  const keywordBox = (await keywords.boundingBox())!;
  const codesBox = (await statusCodes.boundingBox())!;
  if (viewport!.width >= 1280) {
    expect(codesBox.x).toBeGreaterThanOrEqual(keywordBox.x + keywordBox.width);
  } else {
    expect(codesBox.y).toBeGreaterThanOrEqual(keywordBox.y + keywordBox.height);
  }
});

test("窄屏开关与标签保持同排，选择中文处置动作后切换分类保留草稿", async ({ page }) => {
  await page.goto("/policy");
  await page.getByRole("tab", { name: "健康与处置", exact: true }).click();
  const toggle = page.getByRole("switch", { name: "启用降级", exact: true });
  const label = page.getByText("启用降级", { exact: true });
  const toggleBox = (await toggle.boundingBox())!;
  const labelBox = (await label.boundingBox())!;
  expect(toggleBox.x).toBeGreaterThanOrEqual(labelBox.x + labelBox.width);
  expect(toggleBox.y).toBeLessThan(labelBox.y + labelBox.height);
  expect(toggleBox.y + toggleBox.height).toBeGreaterThan(labelBox.y);
  const action = page.getByRole("combobox", { name: "处置动作", exact: true });
  await action.click();
  await page.getByRole("option", { name: "停用账号", exact: true }).click();
  await expect(action).toContainText("停用账号");
  await page.getByRole("tab", { name: "守护范围", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "选择暂停调度的账号" })).toBeEnabled();
  await page.getByRole("tab", { name: "健康与处置", exact: true }).click();
  await expect(action).toContainText("停用账号");
});

test("中等宽度下四个分类与长字段仍完整显示", async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 768 });
  await page.goto("/policy");
  await page.getByRole("tab", { name: "健康与处置", exact: true }).click();
  const fields = page
    .getByRole("region", { name: "健康分公式", exact: true })
    .locator('[data-slot="policy-fields"]');
  await expect(fields).toBeVisible();
  expect(
    await fields.evaluate(
      (element) => getComputedStyle(element).gridTemplateColumns.split(" ").length,
    ),
  ).toBe(2);
  const tabs = page.getByRole("tablist", { name: "策略分类" });
  for (const tab of await tabs.getByRole("tab").all())
    await expect(tab).toBeInViewport({ ratio: 1 });
  const content = page.locator('[data-slot="page-content"]');
  expect(await content.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
});

test("共享并发指定账号可保存稳定 ID，刷新后保留选择且窄屏不溢出", async ({ page }) => {
  let saved = structuredClone(policy);
  await page.route("**/api/policy", async (route) => {
    if (route.request().method() === "PATCH" || route.request().method() === "PUT") {
      const payload = route.request().postDataJSON();
      saved = { ...saved, ...payload };
    }
    await route.fulfill({ json: saved });
  });
  await page.route("**/api/accounts", (route) =>
    route.fulfill({
      json: [
        { id: "41", name: "共享额度账号", upstream_type: "sub2api", groups: ["codex"] },
        { id: "42", name: "独立额度账号", upstream_type: "newapi", groups: [] },
      ],
    }),
  );
  await page.goto("/policy");
  const card = page.getByRole("region", { name: "上游共享并发分配", exact: true });
  await card.getByRole("switch", { name: "启用上游共享并发分配" }).check();
  await card.getByRole("combobox", { name: "共享并发分配范围" }).click();
  await page.getByRole("option", { name: "指定账号", exact: true }).click();
  await card.getByRole("combobox", { name: "参与共享并发分配的账号" }).click();
  await expect(page.getByRole("option", { name: /独立额度账号/ })).toHaveCount(0);
  await page.getByRole("option", { name: /共享额度账号/ }).click();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await expect
    .poll(() => saved.advanced_policy.upstream_concurrency)
    .toEqual({ enabled: true, account_mode: "selected", account_ids: ["41"] });
  await page.reload();
  await expect(card.getByRole("combobox", { name: "共享并发分配范围" })).toContainText("指定账号");
  await expect(card.getByRole("combobox", { name: "参与共享并发分配的账号" })).toContainText(
    "共享额度账号",
  );
  expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
});

test("共享并发指定上游可保存稳定 ID，刷新后保留选择且窄屏不溢出", async ({ page }) => {
  let saved = structuredClone(policy);
  let accountRequests = 0;
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/api/accounts") accountRequests++;
  });
  await page.route("**/api/policy", async (route) => {
    if (route.request().method() === "PATCH" || route.request().method() === "PUT") {
      const payload = route.request().postDataJSON();
      saved = { ...saved, ...payload };
    }
    await route.fulfill({ json: saved });
  });
  await page.route("**/api/upstreams", (route) =>
    route.fulfill({
      json: {
        hosts: [
          {
            upstream_id: "Upstream-A",
            name: "共享额度上游",
            upstream_type: "sub2api",
            host: "a.example",
          },
          {
            upstream_id: "upstream-b",
            name: "独立额度上游",
            upstream_type: "newapi",
            host: "b.example",
          },
        ],
      },
    }),
  );
  await page.goto("/policy");
  const card = page.getByRole("region", { name: "上游共享并发分配", exact: true });
  await card.getByRole("switch", { name: "启用上游共享并发分配" }).check();
  await card.getByRole("combobox", { name: "共享并发分配范围" }).click();
  await page.getByRole("option", { name: "指定上游", exact: true }).click();
  await card.getByRole("combobox", { name: "参与共享并发分配的上游" }).click();
  await expect(page.getByRole("option", { name: /独立额度上游/ })).toHaveCount(0);
  await page.getByRole("option", { name: /共享额度上游/ }).click();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await expect
    .poll(() => saved.advanced_policy.upstream_concurrency)
    .toEqual({ enabled: true, account_mode: "upstreams", upstream_ids: ["Upstream-A"] });
  await page.reload();
  await expect(card.getByRole("combobox", { name: "共享并发分配范围" })).toContainText("指定上游");
  await expect(card.getByRole("combobox", { name: "参与共享并发分配的上游" })).toContainText(
    "共享额度上游",
  );
  expect(accountRequests).toBe(0);
  expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
});

test("并发策略与全局默认策略统一整行卡片及字段列数，详细规则按需展开", async ({ page }) => {
  await page.goto("/policy");
  const reference = page.getByRole("region", { name: "全局默认策略", exact: true });
  const referenceFields = reference.locator('[data-slot="policy-fields"]');
  await expect(reference).toBeVisible();
  const columns = await referenceFields.evaluate(
    (element) => getComputedStyle(element).gridTemplateColumns.split(" ").length,
  );
  const referenceBox = (await reference.boundingBox())!;
  for (const name of ["上游共享并发分配", "智能扩容"]) {
    const card = page.getByRole("region", { name, exact: true });
    const box = (await card.boundingBox())!;
    expect(Math.abs(box.width - referenceBox.width)).toBeLessThanOrEqual(1);
    expect(
      await card
        .locator('[data-slot="policy-fields"]')
        .evaluate((element) => getComputedStyle(element).gridTemplateColumns.split(" ").length),
    ).toBe(columns);
    const details = card.locator("details");
    await expect(details).not.toHaveAttribute("open", "");
    const summary = details.locator("summary");
    await summary.focus();
    await page.keyboard.press("Enter");
    await expect(details).toHaveAttribute("open", "");
    await page.keyboard.press("Enter");
    await expect(details).not.toHaveAttribute("open", "");
    expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
    await card.screenshot({ path: test.info().outputPath(`${name}.png`) });
  }
});
