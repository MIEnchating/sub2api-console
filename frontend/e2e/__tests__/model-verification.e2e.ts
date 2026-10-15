import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import { account } from "../../src/features/accounts/__tests__/fixtures";

test("新模型可直接检测并查看独立身份与能力证据", async ({ page }, testInfo) => {
  const task = {
    id: "verification-task",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "succeeded",
    progress: 100,
    message: "完成",
    created_at: "2026-10-14T00:00:00Z",
    updated_at: "2026-10-14T00:01:00Z",
    result: {
      tests: [
        {
          account_id: "41",
          account_name: "检测账号",
          claimed_model: "new-model",
          checker: "verification",
          version: "verification-v4",
          verdict: "NO_ANOMALIES",
          authenticity: {
            status: "observed",
            summary: "接口检查已完成，返回型号与协议证据见明细。",
          },
          fingerprint: {
            status: "stable",
            samples: [
              {
                id: "baseline",
                label: "基准文本",
                status: "stable",
                first: 41,
                repeat: 41,
                delta: 0,
              },
            ],
          },
          capability: { status: "passed", summary: "本次能力检测通过" },
          total: 2,
          passed: 2,
          requests: { successful: 2, total: 2 },
          evidence: [{ id: "source", label: "请求来源", detail: "第三方接口" }],
          probes: [
            {
              id: "arithmetic",
              label: "多步计算",
              request_id: "req-1",
              round: 1,
              status: "passed",
              prompt: "7 × 3",
              expected: "21",
              answer: "**21**",
              protocol: "openai_responses",
              http_status: 200,
            },
            {
              id: "context",
              label: "上下文信息提取",
              request_id: "req-2",
              round: 1,
              status: "passed",
              prompt: "长题目\n".repeat(120),
              expected: "42",
              answer: "42",
              protocol: "openai_responses",
              http_status: 200,
            },
          ],
        },
      ],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    expect(path).not.toContain("model-checks/configuration");
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/accounts": [{ ...account, id: "41", name: "检测账号", platform: "openai" }],
      "/api/accounts/41/models": { models: ["new-model"] },
      "/api/model-checks": task,
      "/api/tasks/verification-task": task,
      "/api/model-checks/account-statuses": [],
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ status: 200, contentType: "text/event-stream", body: "" });
      return;
    }
    await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/model-check");
  await expect(page.getByRole("button", { name: "检测规则与题库" })).toHaveCount(0);
  await page.getByRole("button", { name: "全选账号" }).click();
  await page.getByRole("checkbox", { name: "选择模型 new-model" }).click();
  await expect(page.getByText(/计划 17 项检测/)).toBeVisible();
  await page.getByRole("button", { name: /开始检测/ }).click();
  const result = page.getByRole("dialog", { name: "模型检测结果", exact: true });
  await expect(result).toBeVisible();
  await expect(result.getByText(/对照证据未记录|官方对照未进行/)).toHaveCount(0);
  const results = result.getByRole("region", { name: "账号模型检测结果" });
  for (const label of ["能力通过", "接口检查完成", "指纹采集稳定"]) {
    const badge = results
      .getByText(label, { exact: true })
      .locator('xpath=ancestor::*[@data-slot="badge"]');
    await expect(badge).toHaveAttribute("data-variant", "success");
    await expect(badge.locator("svg")).toHaveCount(1);
    const alignment = await badge.evaluate((element) => {
      const icon = element.querySelector("svg")!;
      const content = icon.parentElement!;
      const label = icon.nextElementSibling!;
      const iconBox = icon.getBoundingClientRect();
      const labelBox = label.getBoundingClientRect();
      return {
        align: getComputedStyle(content).alignItems,
        gap: labelBox.left - iconBox.right,
        center: Math.abs(iconBox.top + iconBox.height / 2 - labelBox.top - labelBox.height / 2),
      };
    });
    expect(alignment.align).toBe("center");
    expect(alignment.gap).toBeGreaterThan(0);
    expect(alignment.center).toBeLessThanOrEqual(1);
  }
  await expect(results.getByText(/历史报告未记录官方对照是否完成/)).toHaveCount(0);
  await expect(result.getByRole("button", { name: "转到第 1 页" })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("verification-result.png") });
  await result.getByRole("button", { name: "查看检测证据" }).filter({ visible: true }).click();
  const evidence = page.getByRole("dialog", { name: "检测证据", exact: true });
  for (const label of ["能力通过", "接口检查完成", "指纹采集稳定"]) {
    await expect(
      evidence.getByText(label, { exact: true }).locator('xpath=ancestor::*[@data-slot="badge"]'),
    ).toHaveAttribute("data-variant", "success");
  }
  await expect(evidence.getByText("**21**", { exact: true })).toBeVisible();
  await expect(evidence.getByText(/HTTP 200/)).toBeVisible();
  await expect(evidence).toBeInViewport();
  await expect(evidence.getByText("7 × 3", { exact: true })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("verification-evidence.png") });
  const nextProbe = evidence.getByRole("button", { name: /第 1 轮.*上下文信息提取/ });
  await nextProbe.focus();
  await page.keyboard.press("Enter");
  await expect(nextProbe).toHaveAttribute("aria-pressed", "true");
  const content = evidence.getByRole("region", { name: "检测项内容" });
  await content.getByText("请求 ID：", { exact: true }).scrollIntoViewIfNeeded();
  await expect(evidence.getByRole("button", { name: "关闭" })).toBeInViewport({ ratio: 1 });
  await expect(evidence.getByRole("button", { name: "逐项检测结果" })).toBeInViewport({ ratio: 1 });
  expect(
    await evidence.evaluate((element) => element.scrollWidth - element.clientWidth),
  ).toBeLessThanOrEqual(1);
  await evidence.getByRole("button", { name: "来源与指纹" }).click();
  await expect(evidence.getByText("第三方接口", { exact: true })).toBeVisible();
  const sampleStatus = evidence
    .getByRole("cell", { name: "稳定", exact: true })
    .locator('[data-slot="badge"]');
  await expect(sampleStatus).toHaveAttribute("data-variant", "success");
  await expect(sampleStatus.locator("svg")).toHaveCount(1);
  await expect(evidence.getByText(/能力、请求来源与指纹稳定性分别展示/)).toBeVisible();
  await evidence.getByRole("button", { name: "逐项检测结果" }).click();
  await expect(nextProbe).toHaveAttribute("aria-pressed", "true");
  await page.keyboard.press("Escape");
  await expect(evidence).toHaveCount(0);
  await expect(result).toBeVisible();
});

test("多组合报告内部滚动且分页与关闭入口保持可用", async ({ page }) => {
  const tests = Array.from({ length: 21 }, (_, index) => ({
    account_id: String(index + 1),
    account_name: `报告账号 ${index + 1}`,
    claimed_model: "test-model",
    checker: "verification",
    verdict: "NO_ANOMALIES",
    capability: { status: "passed" },
    authenticity: [
      { status: "official_source", summary: "本次固定官方端点已返回有效回答。" },
      { status: "reference_consistent", summary: "全部计划项已完成对照，仅说明本次观测一致。" },
      { status: "evidence_mismatch", summary: "输入用量存在差异，请核对网关提示词。" },
    ][index % 3],
    total: 1,
    passed: 1,
    requests: { successful: 1, total: 1 },
    probes: [
      {
        id: "structured",
        label: "结构化输出",
        round: 1,
        status: "passed",
        prompt: "返回原始 JSON",
        expected: '{"value":9007199254740993}',
        answer: '{"value":9007199254740993}',
        request_id: `req-${index}`,
      },
    ],
  }));
  const task = {
    id: "many-results",
    skill: "sub2api-model-check",
    operation: "account-model-behavior-check",
    status: "succeeded",
    progress: 100,
    message: "完成",
    result: { tests },
    created_at: "2026-10-15T00:00:00Z",
    updated_at: "2026-10-15T00:01:00Z",
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/events")) {
      await route.fulfill({ status: 200, contentType: "text/event-stream", body: "" });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试" },
      "/api/accounts": [{ ...account, platform: "openai" }],
      "/api/accounts/41/models": { models: ["test-model"] },
      "/api/model-checks": task,
      "/api/tasks/many-results": task,
    };
    await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/model-check");
  await page.getByRole("button", { name: "全选账号" }).click();
  await page.getByRole("checkbox", { name: "选择模型 test-model" }).click();
  await page.getByRole("button", { name: /开始检测/ }).click();
  const dialog = page.getByRole("dialog", { name: "模型检测结果", exact: true });
  const results = dialog.getByRole("region", { name: "账号模型检测结果" });
  await expect(results.getByText("官方来源已确认").first()).toBeVisible();
  await expect(results.getByText("对照未见差异").first()).toBeVisible();
  await expect(results.getByText("存在待核对差异").first()).toHaveCount(1);
  await results.getByText("报告账号 20", { exact: false }).scrollIntoViewIfNeeded();
  await expect.poll(() => results.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await expect(dialog.getByRole("button", { name: "关闭" })).toBeInViewport({ ratio: 1 });
  const nextPage = dialog.getByRole("button", { name: "转到下一页" });
  await expect(nextPage).toBeInViewport({ ratio: 1 });
  await nextPage.click();
  await expect(results.getByText("报告账号 21", { exact: false })).toBeVisible();
  await results.getByRole("button", { name: "查看检测证据" }).click();
  const evidence = page.getByRole("dialog", { name: "检测证据", exact: true });
  const actual = evidence.getByRole("region", { name: "实际回答", exact: true });
  await expect(actual.locator(".cm-content")).toHaveText('{"value":9007199254740993}');
  await expect(actual.locator(".cm-content")).toHaveAttribute("contenteditable", "false");
  expect(
    await evidence.evaluate((element) => element.scrollWidth - element.clientWidth),
  ).toBeLessThanOrEqual(1);
  await page.keyboard.press("Escape");
  await expect(dialog.getByRole("button", { name: "查看检测证据" })).toBeFocused();
});
