import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";

import type { AccountRecentResult, AccountStatus } from "../../src/api";

const firstSuccess: AccountRecentResult = {
  id: "1",
  result: "成功",
  event_type: "healthy",
  score: 100,
  observed_at: "2026-09-17T08:00:00Z",
  latency_ms: 120,
  failure_reason: null,
  source: "traffic",
};
const secondSuccess: AccountRecentResult = {
  id: "2",
  result: "成功",
  event_type: "healthy",
  score: 100,
  observed_at: "2026-09-17T08:00:01Z",
  latency_ms: 180,
  failure_reason: null,
  source: "traffic",
};
const clientError: AccountRecentResult = {
  id: "3",
  result: "失败",
  event_type: "client_error",
  score: 0,
  observed_at: "2026-09-17T08:00:02Z",
  latency_ms: null,
  failure_reason: "HTTP 400：请求参数无效，请检查请求内容",
  source: "traffic",
};
const gatewayError: AccountRecentResult = {
  id: "4",
  result: "失败",
  event_type: "gateway_error",
  score: 25,
  observed_at: "2026-09-17T08:00:03Z",
  latency_ms: null,
  failure_reason: "HTTP 503：上游网关错误",
  source: "traffic",
};
const initialAccount: AccountStatus = {
  id: "41",
  name: "实时健康隔离测试账号",
  groups: ["隔离测试"],
  upstream_id: "isolated-upstream",
  upstream_host: "upstream.example.test",
  upstream_type: "newapi",
  schedulable: true,
  priority: 10,
  load_factor: "1",
  concurrency: 5,
  multiplier: "1",
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
  target_priority: 10,
  target_load_factor: "1",
  target_schedulable: true,
  target_concurrency: 5,
  health_score: 99,
  short_score: 99,
  long_score: 99,
  health_evaluated_at: "2026-09-17T08:00:02Z",
  health_evidence_at: "2026-09-17T08:00:01Z",
  sample_count: 2,
  short_sample_count: 2,
  long_sample_count: 2,
  recent_results: [clientError, secondSuccess, firstSuccess],
  ttfb_p50_ms: 120,
  ttfb_p95_ms: 180,
  weight: 80,
};
const healthSnapshot = {
  health_score: 66.25,
  short_score: 62.5,
  long_score: 75,
  sample_count: 3,
  short_sample_count: 3,
  long_sample_count: 3,
  ttfb_p50_ms: 120,
  ttfb_p95_ms: 180,
  health_evaluated_at: "2026-09-17T08:00:04.123456789Z",
  health_evidence_at: "2026-09-17T08:00:03Z",
};

async function mockAccountAPI(page: Page): Promise<{
  releaseSnapshot: () => void;
  unexpectedRequests: string[];
}> {
  let releaseSnapshot = (): void => {};
  const snapshotReady = new Promise<void>((resolve) => {
    releaseSnapshot = resolve;
  });
  const unexpectedRequests: string[] = [];
  await page.addInitScript(() => {
    localStorage.setItem("sub2api-console-theme", "light");
  });
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离测试" },
      "/api/accounts": [initialAccount],
      "/api/model-checks/account-statuses": [],
      "/api/groups": [],
      "/api/dictionaries": { items: [] },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "isolated" },
      "/api/overview": { mode: "监控模式", account_count: 1, group_count: 0, open_alerts: 0 },
      "/api/policy": { advanced_policy: { manual_priority: { reserved_max: 10 } } },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (request.method() === "GET" && path === "/api/accounts/results/events") {
      await snapshotReady;
      await route.fulfill({
        contentType: "text/event-stream",
        body: `retry: 86400000\nevent: snapshot\ndata: ${JSON.stringify({
          account_id: initialAccount.id,
          results: [gatewayError, clientError, secondSuccess, firstSuccess],
          health: healthSnapshot,
        })}\n\n`,
      });
    } else if (request.method() === "GET" && path === "/api/inspection/automation/events") {
      await route.fulfill({
        contentType: "text/event-stream",
        body: "retry: 86400000\n: isolated fixture\n\n",
      });
    } else if (request.method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      unexpectedRequests.push(`${request.method()} ${path}`);
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
  return { releaseSnapshot, unexpectedRequests };
}

test("收到健康SSE快照时新失败色块与健康分从99同步更新为66", async ({ page }) => {
  const mock = await mockAccountAPI(page);
  try {
    await page.goto("/accounts");
    const row = page.getByRole("row").filter({ hasText: initialAccount.name });
    await expect(row.getByLabel("健康分 99", { exact: true })).toBeVisible();
    await expect(row.getByLabel(/网关错误 · 25 分/)).toHaveCount(0);

    mock.releaseSnapshot();

    await expect
      .poll(async () => ({
        score: await row.getByLabel("健康分 66", { exact: true }).count(),
        failure: await row.getByLabel(/网关错误 · 25 分/).count(),
      }))
      .toEqual({ score: 1, failure: 1 });
    await expect(row.getByText("有效样本 3", { exact: true })).toBeVisible();
    await expect(row.getByLabel(/完美健康 · 100 分/)).toHaveCount(2);
    await expect(row.getByLabel(/查看健康评分详情/)).toHaveAccessibleName(
      /短期评分 63，长期评分 75，短期样本数 3，长期样本数 3/,
    );
    await row.getByLabel(/查看健康评分详情/).focus();
    const tooltip = page.getByRole("tooltip", { name: "健康评分详情" });
    await expect(tooltip).toContainText("当前证据评分");
    await expect(tooltip.locator("time").first()).toHaveAttribute(
      "datetime",
      healthSnapshot.health_evaluated_at,
    );
    await expect(tooltip.locator("time").last()).toHaveAttribute(
      "datetime",
      healthSnapshot.health_evidence_at,
    );
    expect(mock.unexpectedRequests).toEqual([]);
    await page.screenshot({ path: test.info().outputPath("live-health-score.png") });
  } finally {
    mock.releaseSnapshot();
  }
});

test("客户端400使用中立提示且不显示零分，真实网关失败仍显示25分", async ({ page }) => {
  const mock = await mockAccountAPI(page);
  try {
    await page.goto("/accounts");
    const row = page.getByRole("row").filter({ hasText: initialAccount.name });
    const clientResult = row.getByLabel(/客户端请求错误（不计入健康评分）/);
    await expect(clientResult).toBeVisible();
    await expect(clientResult).toHaveClass(/bg-muted-foreground\/60/);
    await expect(clientResult).not.toHaveAccessibleName(/\d+ 分/);
    await clientResult.focus();
    await expect(page.getByRole("tooltip")).toContainText("不计入健康评分");
    await expect(page.getByRole("tooltip")).toContainText("HTTP 400");
    await expect(page.getByRole("tooltip")).not.toContainText("0 分");
    await page.keyboard.press("Escape");

    mock.releaseSnapshot();

    const gatewayResult = row.getByLabel(/网关错误 · 25 分/);
    await expect(gatewayResult).toBeVisible();
    const clientColor = await clientResult.evaluate(
      (element) => getComputedStyle(element).backgroundColor,
    );
    const gatewayColor = await gatewayResult.evaluate(
      (element) => getComputedStyle(element).backgroundColor,
    );
    expect(gatewayColor).not.toBe(clientColor);
    await gatewayResult.focus();
    await expect(page.getByRole("tooltip")).toContainText("网关错误 · 25 分");
    await expect(page.getByRole("tooltip")).not.toContainText("不计入健康评分");
    expect(mock.unexpectedRequests).toEqual([]);
  } finally {
    mock.releaseSnapshot();
  }
});
