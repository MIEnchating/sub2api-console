import { expect, test } from "@playwright/test";
import type { AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

// Keep historical fixtures on the displayed Beijing calendar day.
test.beforeEach(async ({ page }) => {
  await page.clock.setFixedTime(new Date("2026-10-10T08:00:00Z"));
});

test("前置与动画失败详情使用一致的紧凑布局，窄屏统计分两列且移除费用", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const result: AnimationResult = {
    account_id: "41",
    account_name: "cicadas-0.04",
    model: "gpt-6-astra",
    status: "failed",
    request_id: "model-check-0db34b4edaebdf2a-887-animation",
    duration_ms: 400,
    completed_at: "2026-10-10T00:31:00Z",
    reasoning_effort: "low",
    error: "生成的 SVG 包含不支持的元素，请重试",
    source: '<svg viewBox="0 0 640 400">\n  <image href="https://example.invalid/image"/>\n</svg>',
  };
  const precheck: AnimationResult = {
    ...result,
    mode: "precheck",
    request_id: "model-check-0db34b4edaebdf2a-887-precheck",
    reasoning_effort: "medium",
    error: "前置检测请求失败，请查看题目结果后重试",
    precheck: {
      verdict: "error",
      profile_version: "astra-v1",
      questions: [
        {
          id: "candy",
          verdict: "error",
          error:
            "上游连接返回 HTTP 400：Upstream rejected illegal short-input distillation or heartbeat probing.",
          request_id: "model-check-0db34b4edaebdf2a-887-precheck-candy",
        },
      ],
    },
  };
  const task: Task = {
    id: "details-layout",
    skill: "sub2api-model-animation",
    operation: "account-model-combined",
    status: "failed",
    progress: 100,
    message: "检测失败",
    result: { mode: "both", account_ids: ["41"], animations: [result, precheck] },
    created_at: "2026-10-10T00:30:00Z",
    updated_at: result.completed_at,
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "详情布局测试" },
      "/api/accounts": [{ ...account, id: "41", name: result.account_name, platform: "openai" }],
      "/api/model-checks/animations": [{ ...task, result: {} }],
      [`/api/tasks/${task.id}`]: task,
      "/api/model-checks/animation-schedules": [],
      "/api/dictionaries": { items: [] },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  for (const stage of ["前置检测", "动画检测"]) {
    await page
      .getByRole("tab", { name: stage === "前置检测" ? "前置检测" : "账号检测", exact: true })
      .click();
    const trigger = page
      .getByRole("article", { name: `账号 ${result.account_name}` })
      .getByRole("button", { name: `查看${stage}详情` });
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: `${stage}详情` });
    const metrics = dialog.getByRole("group", { name: "用量与性能" });
    await expect(metrics).toBeVisible();
    const columns = await metrics.evaluate(
      (element) => getComputedStyle(element).gridTemplateColumns.split(" ").length,
    );
    expect(columns).toBe(page.viewportSize()!.width < 640 ? 2 : 3);
    await expect(dialog.getByText(result.model, { exact: true })).toHaveCount(1);
    await expect(dialog.getByText("耗时", { exact: true })).toHaveCount(1);
    await expect(dialog.getByText(/^请求 ID[：:]?$/)).toHaveCount(1);
    await expect(dialog.getByText(/预估费用|无法估算/)).toHaveCount(0);
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    expect((await dialog.boundingBox())!.width).toBeLessThanOrEqual(768);
    await page.screenshot({ path: testInfo.outputPath(`${stage}-details.png`) });
    const metadata = dialog.getByRole("region", { name: "请求信息" });
    await metadata.scrollIntoViewIfNeeded();
    await expect(metadata).toContainText(
      stage === "前置检测" ? precheck.request_id : result.request_id,
    );
    await expect(dialog.getByRole("heading", { name: `${stage}详情`, exact: true })).toBeInViewport(
      { ratio: 1 },
    );
    await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
      ratio: 1,
    });
    await page.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
  }
});
