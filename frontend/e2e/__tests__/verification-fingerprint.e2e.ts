import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("独立检测只提交所选账号且历史指纹对照仍可读取", async ({ page }, testInfo) => {
  const samples = [
    { id: "baseline", label: "基准文本", first: 117, repeat: 117, delta: 0, status: "stable" },
    { id: "chinese", label: "中文文本", first: 155, repeat: 155, delta: 38, status: "stable" },
    { id: "english", label: "英文文本", first: 141, repeat: 141, delta: 24, status: "stable" },
    { id: "code", label: "代码文本", first: 159, repeat: 159, delta: 42, status: "stable" },
    { id: "unicode", label: "Unicode 文本", first: 167, repeat: 167, delta: 50, status: "stable" },
  ];
  const task = {
    id: "fingerprint-task",
    skill: "sub2api-model-check",
    status: "succeeded",
    progress: 100,
    message: "完成",
    created_at: "2026-10-15T00:00:00Z",
    updated_at: "2026-10-15T00:01:00Z",
    result: {
      tests: [
        {
          account_id: "41",
          account_name: "待测接口",
          claimed_model: "new-model",
          checker: "verification",
          version: "verification-v3",
          verdict: "NO_ANOMALIES",
          capability: { status: "passed" },
          total: 17,
          passed: 7,
          capability_total: 7,
          requests: { successful: 17, total: 17 },
          reference: { status: "mismatch", summary: "中文差分与官方参考不同，请核对逐项数值。" },
          fingerprint: {
            status: "stable",
            summary: "5 类文本各测 2 次，已减去基准用量。",
            samples,
          },
          reference_comparisons: [
            {
              account_id: "42",
              status: "mismatch",
              summary: "中文差分不同，不据此断言假模型。",
              samples: [{ label: "中文文本", observed: 38, reference: 30, difference: 8 }],
            },
          ],
          probes: [
            {
              id: "fingerprint_chinese",
              label: "中文文本采样",
              round: 1,
              category: "fingerprint",
              status: "observed",
              answer: "OK",
              expected: "采集输入用量",
              prompt: "测量文本",
              usage: { input_tokens: 155 },
              request_id: "sample-1",
            },
          ],
        },
      ],
    },
  };
  let submitted: unknown;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/events")) {
      await route.fulfill({ status: 200, contentType: "text/event-stream", body: "" });
      return;
    }
    if (path === "/api/model-checks" && route.request().method() === "POST")
      submitted = route.request().postDataJSON();
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/accounts": [
        {
          ...account,
          id: "41",
          name: "待测接口",
          platform: "openai",
          account_type: "apikey",
          base_url: "https://relay.example.invalid",
        },
        {
          ...account,
          id: "42",
          name: "官方参考",
          platform: "openai",
          account_type: "apikey",
          base_url: "https://api.openai.com/v1",
        },
      ],
      "/api/accounts/41/models": { models: ["new-model"] },
      "/api/accounts/42/models": { models: ["new-model"] },
      "/api/model-checks": task,
      "/api/tasks/fingerprint-task": task,
    };
    await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/model-check?account_id=41");
  await expect(page.getByRole("checkbox", { name: "采集分词指纹", exact: true })).toBeChecked();
  await expect(page.getByText(/官方 API 对照（可选）/)).toHaveCount(0);
  await expect(page.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  const model = page.getByRole("checkbox", { name: "选择模型 new-model" });
  await model.check();
  await model.uncheck();
  await expect(page.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  await expect(page.getByText(/请选择至少一个账号|请选择至少一个模型/)).toHaveCount(0);
  await page.getByRole("button", { name: "清空选择" }).click();
  await expect(page.getByText(/请选择至少一个账号|请选择至少一个模型/)).toHaveCount(0);
  await expect(page.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  await page.getByRole("button", { name: "全选账号" }).click();
  await model.check();
  await expect(page.getByText(/计划 17 项检测/)).toBeVisible();
  await page.getByRole("button", { name: /开始检测/ }).click();
  await expect
    .poll(() => submitted)
    .toMatchObject({ account_ids: ["41"], models: ["new-model"], fingerprint: true });
  const result = page.getByRole("dialog", { name: "模型检测结果", exact: true });
  await expect(result.getByText("官方对照存在差异")).toBeVisible();
  await expect(result.getByText("指纹采集稳定")).toBeVisible();
  await expect(result.getByText(/题目通过 7\/7/)).toBeVisible();
  await result.getByRole("button", { name: "查看检测证据" }).click();
  const evidence = page.getByRole("dialog", { name: "检测证据", exact: true });
  await expect(evidence.getByText("采样目的", { exact: true })).toBeVisible();
  await evidence.getByRole("button", { name: "来源与指纹" }).click();
  const sampling = evidence.getByRole("region", { name: "分词指纹采样" });
  await expect(sampling.getByRole("row", { name: /中文文本/ })).toContainText("155");
  const comparison = evidence.getByRole("region", { name: "官方指纹对照明细" });
  await comparison.scrollIntoViewIfNeeded();
  const row = comparison.getByRole("row", { name: /中文文本/ });
  await expect(row.getByRole("cell")).toHaveText(["38", "30", "8"]);
  await expect(evidence.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(
    await evidence.evaluate((element) => element.scrollWidth - element.clientWidth),
  ).toBeLessThanOrEqual(1);
  await page.screenshot({ path: testInfo.outputPath("fingerprint-comparison.png") });
  await page.keyboard.press("Escape");
  await expect(result.getByRole("button", { name: "查看检测证据" })).toBeFocused();
});
