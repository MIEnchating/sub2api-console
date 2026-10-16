import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("检测历史可翻页并查看证据，长记录在弹窗内部滚动", async ({ page }, testInfo) => {
  const summary = {
    id: "history-21",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "succeeded",
    progress: 100,
    message: "历史检测完成",
    created_at: "2026-10-15T00:00:00Z",
    updated_at: "2026-10-15T00:01:00Z",
  };
  let detailsRead = 0;
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/model-checks/history") {
      const items =
        url.searchParams.get("page") === "2"
          ? [summary]
          : Array.from({ length: 20 }, (_, index) => ({
              ...summary,
              id: `history-${index}`,
              message: "多账号模型检测记录".repeat(12),
            }));
      await route.fulfill({ json: { items, has_more: url.searchParams.get("page") === "1" } });
      return;
    }
    if (url.pathname === "/api/tasks/history-21") {
      detailsRead++;
      await route.fulfill({
        json: {
          ...summary,
          result: {
            tests: [
              {
                checker: "verification",
                account_name: "历史账号",
                claimed_model: "new-model",
                capability: { status: "passed" },
                authenticity: { status: "official_source", summary: "本次官方来源已确认。" },
                probes: [
                  {
                    id: "math",
                    label: "计算",
                    round: 1,
                    status: "passed",
                    prompt: "7 × 3",
                    expected: "21",
                    answer: "**21**",
                  },
                  {
                    id: "public_kbf_crypto_0",
                    category: "public_reference",
                    label: "知识边界 · 加密参数",
                    round: 1,
                    status: "error",
                    error: "请求超时",
                    prompt: "测试知识题",
                  },
                ],
              },
            ],
          },
        },
      });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/accounts": [],
      "/api/model-checks/account-statuses": [],
    };
    await route.fulfill({ json: fixtures[url.pathname] ?? [] });
  });
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测历史", exact: true }).click();
  const history = page.getByRole("dialog", { name: "检测历史", exact: true });
  const list = history.getByRole("region", { name: "模型检测历史列表" });
  await expect(list.getByText("任务 ID：history-19", { exact: true })).toBeAttached();
  await list.getByText("任务 ID：history-19", { exact: true }).scrollIntoViewIfNeeded();
  expect(await list.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  await expect(history.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await expect(history.getByRole("button", { name: "下一页", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(await history.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
  await history.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(history.getByText("历史检测完成", { exact: true })).toBeVisible();
  expect(detailsRead).toBe(0);
  await history.getByRole("button", { name: "查看结果", exact: true }).click();
  const result = page.getByRole("dialog", { name: "历史检测结果", exact: true });
  await expect(
    result.getByRole("region", { name: "账号模型检测结果" }).getByText(/历史账号/),
  ).toBeVisible();
  await result.getByRole("button", { name: "查看检测证据" }).click();
  const evidence = page.getByRole("dialog", { name: "检测证据", exact: true });
  await expect(evidence.getByText("**21**", { exact: true })).toBeVisible();
  const navigation = evidence.getByRole("navigation", { name: "检测项" });
  if ((page.viewportSize()?.width ?? 1440) < 768) {
    await expect(navigation).toBeHidden();
    await evidence.getByRole("combobox", { name: "选择检测项" }).click();
    await page.getByRole("option", { name: /知识边界 · 加密参数/ }).click();
  } else {
    await expect(navigation).toBeVisible();
    await navigation.getByRole("button", { name: /知识边界 · 加密参数/ }).focus();
    await page.keyboard.press("Enter");
  }
  await expect(
    evidence.getByRole("region", { name: "检测项内容" }).getByText("请求超时"),
  ).toBeVisible();
  expect(await evidence.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("grouped-verification-evidence.png") });
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await expect(history.getByText("第 2 页 · 每页 20 条")).toBeVisible();
  await history.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(page.getByRole("button", { name: "检测历史", exact: true })).toBeFocused();
});
