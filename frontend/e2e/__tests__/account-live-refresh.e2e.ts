import { expect, test } from "@playwright/test";
import type { AccountStatus, ModelCheckAccountStatus } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "EventSource", {
      value: class extends EventTarget {
        private listener: (event: Event) => void;
        constructor(url: string) {
          super();
          this.listener = (event) => {
            if (url.includes("/accounts/results/events")) {
              this.dispatchEvent(
                new MessageEvent("snapshot", { data: (event as CustomEvent<string>).detail }),
              );
            }
          };
          window.addEventListener("test-account-snapshot", this.listener);
        }
        close(): void {
          window.removeEventListener("test-account-snapshot", this.listener);
        }
      },
    });
  });
});

test("实时评分更新后勾选与键盘焦点保留，搜索仍显示最新评分", async ({ page }) => {
  const rows = [account, { ...account, id: "42", name: "第二个隔离账号" }];
  await mockAPI(page, () => rows);
  await page.goto("/accounts");
  const checkbox = page.getByRole("checkbox", {
    name: `选择账号 ${account.name}（#41）`,
    exact: true,
  });
  await checkbox.check();
  await checkbox.focus();
  await page.evaluate(() =>
    window.dispatchEvent(
      new CustomEvent("test-account-snapshot", {
        detail: JSON.stringify({
          account_id: "42",
          results: [],
          health: {
            health_score: 70,
            short_score: 70,
            long_score: 70,
            sample_count: 1,
            short_sample_count: 1,
            long_sample_count: 1,
            ttfb_p50_ms: 100,
            ttfb_p95_ms: 200,
            health_evaluated_at: "2026-09-18T08:00:00Z",
            health_evidence_at: null,
          },
        }),
      }),
    ),
  );
  await expect(page.getByLabel("健康分 70", { exact: true })).toBeVisible();
  await expect(checkbox).toBeChecked();
  await expect(checkbox).toBeFocused();
  await page.getByPlaceholder("搜索账号、ID、Host 或分组").fill("第二个隔离账号");
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await expect(page.getByLabel("健康分 70", { exact: true })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("live-refresh.png") });
});

test("账号本身未变化时刷新模型检测结果仍更新置信度", async ({ page }) => {
  let checks: ModelCheckAccountStatus[] = [];
  await mockAPI(
    page,
    () => [account],
    () => checks,
  );
  await page.goto("/accounts");
  await expect(page.getByLabel("置信度短期：暂无样本")).toBeVisible();
  checks = [
    {
      account_id: account.id,
      status: "consistent",
      checked_at: "2026-09-18T08:00:00Z",
      task_id: "isolated-check",
      confidence: {
        evaluated_at: "2026-09-18T08:00:00Z",
        short: { score: 70, samples: 10, passed: 10, failed: 0, inconclusive: 0 },
        long: { score: 80, samples: 20, passed: 20, failed: 0, inconclusive: 0 },
      },
    },
  ];
  await page.getByRole("button", { name: "刷新账号池", exact: true }).click();
  await expect(page.getByLabel("置信度短期：70.0%")).toBeVisible();
  await expect(page.getByLabel("置信度长期：80.0%")).toBeVisible();
});

test("其他账号占用优先位后打开弹窗读取最新占用情况", async ({ page }) => {
  let rows = [account, { ...account, id: "42", name: "占位隔离账号" }];
  await mockAPI(page, () => rows);
  await page.goto("/accounts");
  await expect(page.getByRole("checkbox", { name: /选择账号 占位隔离账号/ })).toBeVisible();
  rows = [account, { ...rows[1], manual_priority: 2 }];
  await page.getByRole("button", { name: "刷新账号池", exact: true }).click();
  await expect(page.getByRole("checkbox", { name: /选择账号 占位隔离账号/ })).toHaveCount(0);
  const row = page.getByRole("row").filter({
    has: page.getByRole("checkbox", { name: `选择账号 ${account.name}（#41）`, exact: true }),
  });
  await row.getByRole("button", { name: "设置手动控制", exact: true }).click();
  await page.getByRole("combobox", { name: "选择手动控制" }).click();
  await expect(
    page.getByRole("option", { name: "2 · 已被 占位隔离账号（42）占用", exact: true }),
  ).toBeDisabled();
});

async function mockAPI(
  page: import("@playwright/test").Page,
  accounts: () => AccountStatus[],
  checks: () => ModelCheckAccountStatus[] = () => [],
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "隔离测试" },
      "/api/accounts": accounts(),
      "/api/model-checks/account-statuses": checks(),
      "/api/groups": [],
      "/api/dictionaries": { items: [] },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "isolated" },
      "/api/overview": { mode: "监控模式", account_count: 2, group_count: 0, open_alerts: 0 },
      "/api/policy": { advanced_policy: { manual_priority: { reserved_max: 10 } } },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (route.request().method() === "GET" && path in responses) {
      await route.fulfill({ json: responses[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
}
