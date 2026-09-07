import { expect, test } from "@playwright/test";

import type { AccountStatus } from "../../src/api";

function account(id: string, overrides: Partial<AccountStatus> = {}): AccountStatus {
  return {
    id,
    name: `布局测试账号 ${id}`,
    groups: ["codex-平价"],
    upstream_id: "upstream-1",
    upstream_host: "upstream.example.test",
    upstream_type: "newapi",
    sub2api_status: "active",
    sub2api_error: null,
    schedulable: true,
    priority: 993,
    load_factor: "57",
    concurrency: 3000,
    multiplier: "0.1",
    balance: "10",
    paused: false,
    paused_reason: null,
    routing_state: "healthy",
    health_status: "healthy",
    health: "healthy",
    desired_health: "healthy",
    apply_pending: false,
    apply_error: null,
    decision_state: "healthy",
    decision_reason: null,
    failure_streak: 0,
    recovery_pass_streak: 2,
    target_priority: 993,
    target_load_factor: "57",
    target_schedulable: true,
    target_concurrency: 3000,
    health_score: 100,
    short_score: 100,
    long_score: 100,
    sample_count: 2,
    recent_results: [],
    ttfb_p50_ms: null,
    ttfb_p95_ms: null,
    weight: 16.8,
    last_error: `上游网关异常：${"service-unavailable/".repeat(30)}`,
    ...overrides,
  };
}

const accounts = [
  account("41", {
    sub2api_status: "error",
    sub2api_error: `上游网关异常：${"service-unavailable/".repeat(30)}`,
    ttfb_p50_ms: 320,
    ttfb_p95_ms: 1250,
    recent_results: Array.from({ length: 10 }, (_, index) => ({
      result: "通过",
      event_type: index < 2 ? "slow" : "healthy",
      score: index < 2 ? 65 : 100,
      observed_at: `2026-09-07T12:${String(59 - index).padStart(2, "0")}:00Z`,
      latency_ms: 320 + index,
      failure_reason: null,
      source: index % 2 === 0 ? "traffic" : "active-probe",
    })),
  }),
  account("42", {
    schedulable: false,
    health_score: 70.8,
    short_score: 62.4,
    long_score: 90.5,
    sample_count: 39,
    recent_results: Array.from({ length: 10 }, (_, index) => ({
      result: "失败",
      event_type: "gateway_error",
      score: 25,
      observed_at: `2026-09-07T12:${String(59 - index).padStart(2, "0")}:00Z`,
      latency_ms: null,
      failure_reason: "上游网关错误",
      source: "traffic",
    })),
  }),
  account("43", {
    name: "超长账号名称".repeat(30),
    health_score: 72.5,
    health: "degraded",
    health_status: "degraded",
    routing_state: "degraded",
    recovery: {
      evaluated_at: "2026-09-07T12:00:00Z",
      ready: false,
      conditions: [
        { code: "health_score", met: false, detail: "健康分尚未达到恢复要求".repeat(30) },
      ],
    },
  }),
];

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript((theme) => {
    localStorage.setItem("sub2api-console-theme", theme ?? "light");
  }, colorScheme);
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/accounts": accounts,
      "/api/groups": [],
      "/api/policy": { advanced_policy: { manual_priority: { reserved_max: 10 } } },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    } else if (route.request().method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("宽屏下状态文案和不同数量的操作按钮均保持在各自列内", async ({ page }) => {
  await page.setViewportSize({ width: 1920, height: 959 });
  await page.goto("/accounts");
  const rows = page.locator("tbody tr");
  await expect(rows).toHaveCount(accounts.length);
  await expect(page.getByRole("columnheader", { name: "Key 状态", exact: true })).toHaveCount(0);
  await expect(page.getByRole("columnheader", { name: "Sub2API 状态", exact: true })).toHaveCount(
    0,
  );
  await expect(rows.first().getByRole("cell")).toHaveCount(10);
  await expect(
    page.getByRole("button", { name: "状态与处置", exact: true }).first(),
  ).toBeInViewport({
    ratio: 1,
  });
  await page.evaluate(() => document.fonts.ready);
  const overflow = await rows.evaluateAll((elements) =>
    elements.flatMap((row, rowIndex) =>
      [8, 9].flatMap((column) => {
        const cell = row.children[column];
        const bounds = cell.getBoundingClientRect();
        return Array.from(cell.querySelectorAll("button, .truncate"))
          .filter((element) => {
            const rect = element.getBoundingClientRect();
            return rect.width > 0 && (rect.left < bounds.left || rect.right > bounds.right);
          })
          .map((element) => ({ row: rowIndex, column, label: element.textContent }));
      }),
    ),
  );
  expect(overflow).toEqual([]);
  await page.screenshot({ path: test.info().outputPath("accounts-layout.png") });
  const clip = await page.locator("table").evaluate((table) => {
    const first = table.querySelector("thead tr")!.children[2].getBoundingClientRect();
    const last = table.querySelector("tbody tr:last-child")!.children[4].getBoundingClientRect();
    return {
      x: first.x,
      y: first.y,
      width: last.right - first.left,
      height: last.bottom - first.top,
    };
  });
  await page.screenshot({ path: test.info().outputPath("account-metrics.png"), clip });
});

test("窄视口仅在表格内横向滚动且可打开完整状态详情", async ({ page }) => {
  await page.goto("/accounts");
  await expect(page.locator("tbody tr")).toHaveCount(accounts.length);
  const container = page.locator('[data-slot="table-container"]');
  await expect(container).toHaveCSS("overflow-x", "auto");
  expect(await container.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
    true,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await container.evaluate((element) => {
    element.scrollLeft = element.scrollWidth;
  });
  const trigger = page.getByRole("button", { name: "状态与处置", exact: true }).first();
  await expect(trigger).toBeInViewport({ ratio: 1 });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "状态与处置" });
  await expect(dialog).toBeVisible();
  await expect(
    dialog.getByText(`最近错误：${accounts[0].sub2api_error}`, { exact: true }),
  ).toBeVisible();
});

test("评分恢复圆环并保留整数，短长期在右侧、样本数在下方", async ({ page }) => {
  await page.goto("/accounts");
  const health = page.locator('[data-slot="account-health-score"]').first();
  await health.scrollIntoViewIfNeeded();
  const score = health.getByLabel("健康分 100", { exact: true });
  await expect(score).toHaveCSS("width", "40px");
  await expect(score.getByText("100", { exact: true })).toHaveCSS("font-size", "11px");
  await expect(score.locator("svg")).toHaveCount(1);
  await expect(health.getByText("有效样本 2", { exact: true })).toBeVisible();
  const bounds = await score.boundingBox();
  const shortBounds = await health.getByLabel("短期评分 100").boundingBox();
  expect(shortBounds!.x).toBeGreaterThanOrEqual(bounds!.x + bounds!.width);
  const samples = await health.getByText("有效样本 2", { exact: true }).boundingBox();
  expect(samples!.y).toBeGreaterThanOrEqual(bounds!.y + bounds!.height);
  await expect(page.getByLabel("健康分 71", { exact: true })).toHaveText("71");
  await expect(page.getByLabel("短期评分 62", { exact: true })).toBeVisible();
  await expect(page.getByLabel("长期评分 91", { exact: true })).toBeVisible();
  expect(
    await health.evaluate((element) => {
      const cell = element.closest("td")!.getBoundingClientRect();
      const rect = element.getBoundingClientRect();
      return rect.left >= cell.left && rect.right <= cell.right;
    }),
  ).toBe(true);
});

test("账号状态文字在表格背景上满足正文对比度", async ({ page }) => {
  await page.goto("/accounts");
  await expect(page.locator("tbody tr")).toHaveCount(accounts.length);
  const badges = page.locator('tbody [data-slot="status-badge"]');
  await expect(badges.first()).toBeVisible();
  const unreadable = await badges.evaluateAll((elements) => {
    const context = document.createElement("canvas").getContext("2d");
    if (!context) throw new Error("Canvas is unavailable");
    const luminance = (): number =>
      Array.from(context.getImageData(0, 0, 1, 1).data)
        .slice(0, 3)
        .map((value) => value / 255)
        .map((value) => (value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4))
        .reduce((total, value, index) => total + value * [0.2126, 0.7152, 0.0722][index], 0);
    return elements.flatMap((element) => {
      context.clearRect(0, 0, 1, 1);
      const ancestors: Element[] = [];
      for (let parent: Element | null = element; parent; parent = parent.parentElement) {
        ancestors.unshift(parent);
      }
      for (const parent of ancestors) {
        context.fillStyle = getComputedStyle(parent).backgroundColor;
        context.fillRect(0, 0, 1, 1);
      }
      const background = luminance();
      context.fillStyle = getComputedStyle(element).color;
      context.fillRect(0, 0, 1, 1);
      const foreground = luminance();
      const ratio =
        (Math.max(foreground, background) + 0.05) / (Math.min(foreground, background) + 0.05);
      return ratio < 4.5 ? [{ label: element.textContent, ratio }] : [];
    });
  });
  expect(unreadable).toEqual([]);
});

test("流量与探针分行显示实心色块，十条同源结果也不越列", async ({ page }) => {
  await page.goto("/accounts");
  const row = page.locator("tbody tr").first();
  const results = row.getByRole("group", { name: "最近结果", exact: true });
  await results.scrollIntoViewIfNeeded();
  const traffic = results.getByLabel(/首字 320ms.*真实流量/);
  const probe = results.getByLabel(/首字 321ms.*探针/);
  await expect(traffic).toHaveCSS("width", "6px");
  await expect(probe).toHaveCSS("height", "12px");
  await expect(probe).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(traffic).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  expect(await traffic.evaluate((element) => getComputedStyle(element).backgroundColor)).toBe(
    await probe.evaluate((element) => getComputedStyle(element).backgroundColor),
  );
  const trafficBounds = await results.getByRole("group", { name: "真实流量结果" }).boundingBox();
  const probeBounds = await results.getByRole("group", { name: "探针结果" }).boundingBox();
  expect(probeBounds!.y).toBeGreaterThanOrEqual(trafficBounds!.y + trafficBounds!.height);
  await expect(row.getByText("320ms", { exact: true })).toBeVisible();
  await expect(row.getByText("1.25s", { exact: true })).toBeVisible();
  await expect(page.getByText("暂无首字数据").first()).toBeVisible();
  const overflow = await page.locator("tbody tr").evaluateAll((rows) =>
    rows.flatMap((element) =>
      [2, 3, 4].filter((index) => {
        const cell = element.children[index];
        const bounds = cell.getBoundingClientRect();
        return Array.from(
          cell.querySelectorAll("[tabindex], [data-slot=account-health-score]"),
        ).some((item) => {
          const rect = item.getBoundingClientRect();
          return rect.left < bounds.left || rect.right > bounds.right;
        });
      }),
    ),
  );
  expect(overflow).toEqual([]);
  await results.getByLabel(/首字 323ms.*探针/).focus();
  await page.keyboard.press("Tab");
  await expect(probe).toBeFocused();
  await expect(page.getByRole("tooltip")).toContainText("探针");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("tooltip")).not.toBeVisible();
});

test("延迟为空时键盘打开说明，可读取统计口径和检查步骤", async ({ page }) => {
  await page.goto("/accounts");
  const trigger = page.getByLabel("真实流量首字延迟说明", { exact: true }).nth(1);
  await trigger.scrollIntoViewIfNeeded();
  await trigger.focus();
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Tab");
  await expect(trigger).toBeFocused();
  const tooltip = page.getByRole("tooltip", { name: /^真实流量首字延迟/ });
  await expect(tooltip).toContainText("探针和请求总耗时不计入");
  await expect(tooltip).toContainText("按有效首字样本最多的模型统计");
  await expect(tooltip).toContainText("多分组账号展示各分组分位数的均值");
  await expect(tooltip).toContainText("流量采集");
  await expect(tooltip).toContainText("下一轮调度评估");
  await expect(trigger).toHaveAccessibleDescription(/真实流量首字延迟/);
  await page.keyboard.press("Escape");
  await expect(tooltip).not.toBeVisible();
});

test("没有流量或探针时显示十格空色条且不增加键盘停靠点", async ({ page }) => {
  await page.goto("/accounts");
  const emptyRow = page.locator("tbody tr").nth(2);
  const results = emptyRow.getByRole("group", { name: "最近结果", exact: true });
  await results.scrollIntoViewIfNeeded();
  await expect(results.getByText(/暂无/)).toHaveCount(0);
  for (const name of ["真实流量无结果", "探针无结果"]) {
    const strip = results.getByRole("img", { name });
    await expect(strip).toBeVisible();
    await expect(strip.locator('[aria-hidden="true"]')).toHaveCount(10);
    await expect(strip.locator('[aria-hidden="true"]').first()).toHaveCSS("height", "12px");
    await expect(strip.locator('[tabindex="0"]')).toHaveCount(0);
  }
  await expect(
    page.locator("tbody tr").nth(1).getByRole("img", { name: "探针无结果" }),
  ).toBeVisible();
});

test("移除Sub2API和Key状态列后加载和空列表均跨越剩余十列", async ({ page }) => {
  let releaseResponse = (): void => {};
  const responseGate = new Promise<void>((resolve) => {
    releaseResponse = resolve;
  });
  await page.route("**/api/accounts", async (route) => {
    await responseGate;
    await route.fulfill({ json: [] });
  });
  await page.goto("/accounts");
  await expect(page.getByRole("columnheader")).toHaveCount(10);
  await expect(page.locator('tbody td[colspan="10"]')).toHaveCount(6);
  releaseResponse();
  await expect(page.locator("tbody td")).toHaveCount(1);
  await expect(page.locator("tbody td")).toHaveAttribute("colspan", "10");
  await expect(page.locator("tbody td")).toContainText("当前没有账号");
});

test("评分详情展示本轮短长期实际样本数，键盘可打开并关闭", async ({ page }) => {
  await page.route("**/api/accounts", (route) =>
    route.fulfill({
      json: [account("14", { sample_count: 58, short_sample_count: 10, long_sample_count: 58 })],
    }),
  );
  await page.goto("/accounts");
  const trigger = page.getByLabel(/^查看健康评分详情/);
  await trigger.scrollIntoViewIfNeeded();
  await trigger.focus();
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Tab");
  await expect(trigger).toBeFocused();
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toContainText("健康评分详情");
  for (const [label, count] of [
    ["短期样本数", "10"],
    ["长期样本数", "58"],
  ]) {
    const value = tooltip
      .locator("dl > div")
      .filter({ has: page.getByText(label, { exact: true }) });
    await expect(value.locator("dd")).toHaveText(count);
  }
  await expect(tooltip).toContainText("不重复相加");
  const bounds = await tooltip.boundingBox();
  expect(bounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width - 32);
  await page.keyboard.press("Escape");
  await expect(tooltip).not.toBeVisible();
});

test("刷新账号快照后错误随Sub2API状态显示与清除，详情不再展示历史失败", async ({ page }) => {
  let snapshot = account("51", {
    sub2api_status: "error",
    sub2api_error: "额度不足，请检查上游账户余额",
    last_error: "历史网关错误",
    recent_results: [
      {
        result: "失败",
        source: "active-probe",
        observed_at: "2026-09-07T12:00:00Z",
        latency_ms: null,
        failure_reason: "历史探针失败",
      },
    ],
  });
  await page.route("**/api/accounts", (route) => route.fulfill({ json: [snapshot] }));
  await page.goto("/accounts");
  const row = page.locator("tbody tr");
  await expect(
    row.getByText("最近错误：额度不足，请检查上游账户余额", { exact: true }),
  ).toBeVisible();
  await expect(row.getByText("调度开关：已开启", { exact: true })).toBeVisible();
  await expect(row.getByText(/最近错误：历史/)).toHaveCount(0);
  const action = page.getByRole("button", { name: "状态与处置", exact: true });
  await action.click();
  const dialog = page.getByRole("dialog", { name: "状态与处置" });
  await expect(
    dialog.getByText("最近错误：额度不足，请检查上游账户余额", { exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();

  snapshot = { ...snapshot, sub2api_status: "active" };
  const refresh = page.getByRole("button", { name: "刷新账号池", exact: true });
  await refresh.click();
  await expect(row.getByText(/^最近错误：/)).toHaveCount(0);
  await action.click();
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText(/^最近错误：/)).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();

  snapshot = { ...snapshot, sub2api_status: "error", sub2api_error: "凭据失效，请更新上游凭据" };
  await refresh.click();
  await expect(row.getByText("最近错误：凭据失效，请更新上游凭据", { exact: true })).toBeVisible();
  await expect(
    row.getByText("最近错误：额度不足，请检查上游账户余额", { exact: true }),
  ).toHaveCount(0);
});
