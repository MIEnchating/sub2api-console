import { expect, test } from "@playwright/test";
import type { AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

// Keep historical fixtures on the displayed Beijing calendar day.
test.beforeEach(async ({ page }) => {
  await page.clock.setFixedTime(new Date("2026-10-10T08:00:00Z"));
});

test("动画校验失败后可查看原文、键盘滚动且不执行生成代码", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const source =
    '<svg xmlns="http://www.w3.org/2000/svg">\n<script>document.title="生成代码被执行"</script>\n' +
    '<image href="https://example.invalid/animation-image"/>\n' +
    "<text>生成的非法 SVG 内容</text>\n".repeat(150) +
    "原文结尾\n</svg>";
  const result: AnimationResult = {
    account_id: "41",
    account_name: "失败原文测试账号",
    model: "gpt-6-astra",
    request_id: "invalid-svg",
    status: "failed",
    error: "生成的 SVG 包含不支持的元素，请重试",
    source,
    source_truncated: true,
    duration_ms: 1200,
    completed_at: "2026-10-10T00:01:00Z",
  };
  const task: Task = {
    id: "animation-failed-source",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "failed",
    progress: 100,
    message: "动画生成失败",
    result: { account_ids: ["41"], animations: [result] },
    created_at: "2026-10-10T00:00:00Z",
    updated_at: result.completed_at,
  };
  const externalRequests: string[] = [];
  await page.route("https://example.invalid/**", async (route) => {
    externalRequests.push(route.request().url());
    await route.abort();
  });
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "失败原文测试" },
      "/api/accounts": [{ ...account, id: "41", name: result.account_name, platform: "openai" }],
      "/api/model-checks/animations": [{ ...task, result: {} }],
      [`/api/tasks/${task.id}`]: task,
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/account-statuses": [],
      "/api/dictionaries": { items: [] },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  const card = page.getByRole("article", { name: `账号 ${result.account_name}` });
  await expect(card.getByText("动画生成失败", { exact: true })).toBeVisible();
  const trigger = card.getByRole("button", { name: "查看动画检测详情" });
  await trigger.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "动画检测详情" });
  const content = dialog.getByRole("region", { name: "生成原文内容" });
  await expect(content).toBeVisible();
  expect(await content.textContent()).toBe(source);
  await expect(dialog.getByRole("region", { name: "失败原因" })).toContainText(result.error!);
  await expect(dialog.getByText(/仅展示前 128 KB/)).toBeVisible();
  await expect(content.locator("script, iframe, svg, image")).toHaveCount(0);
  await expect(page).not.toHaveTitle("生成代码被执行");
  expect(externalRequests).toEqual([]);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await content.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(
    true,
  );
  await content.focus();
  await page.keyboard.press("Control+End");
  await expect.poll(() => content.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await page.screenshot({ path: testInfo.outputPath("failed-animation-source.png") });
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await expect(trigger).toBeFocused();
});
