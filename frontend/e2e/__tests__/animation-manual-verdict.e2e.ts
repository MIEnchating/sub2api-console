import { expect, test } from "@playwright/test";
import type { AnimationResult, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

// Keep historical fixtures on the displayed Beijing calendar day.
test.beforeEach(async ({ page }) => {
  await page.clock.setFixedTime(new Date("2026-10-08T08:00:00Z"));
});

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

test("动画卡片支持标记和清除，详情不提供标记操作，刷新保留且重测不继承", async ({
  page,
}, testInfo) => {
  let result: AnimationResult = {
    account_id: "41",
    account_name: "动画标记测试账号",
    model: "gpt-6-astra",
    request_id: "animation-request-1",
    status: "succeeded",
    duration_ms: 1200,
    completed_at: "2026-10-08T00:01:00Z",
    svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><rect width="640" height="400" fill="#e0f2fe"/><circle cx="320" cy="200" r="75" fill="#059669"/></svg>',
  };
  let task: Task = {
    id: "animation-1",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "succeeded",
    progress: 100,
    message: "检测完成",
    result: { account_ids: ["41"], animations: [result] },
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:01:00Z",
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (
      route.request().method() === "PUT" &&
      path === `/api/model-checks/animations/${task.id}/manual-verdict`
    ) {
      const payload = route.request().postDataJSON();
      expect(payload.account_id).toBe("41");
      expect(payload.request_id).toBe(result.request_id);
      result = {
        ...result,
        manual_verdict: {
          verdict: payload.verdict,
          updated_at: "2026-10-08T01:00:00Z",
          updated_by: "测试用户",
        },
      };
      task = { ...task, result: { ...task.result, animations: [result] } };
      await route.fulfill({ json: task });
      return;
    }
    if (route.request().method() === "POST" && path === "/api/model-checks/animations") {
      result = {
        ...result,
        manual_verdict: undefined,
        request_id: "animation-request-2",
        completed_at: "2026-10-08T02:01:00Z",
      };
      task = {
        ...task,
        id: "animation-2",
        created_at: "2026-10-08T02:00:00Z",
        result: { account_ids: ["41"], animations: [result] },
      };
      await route.fulfill({ json: task });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "测试用户" },
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
  const card = page.getByRole("article", { name: "账号 动画标记测试账号" });
  const actions = card.getByRole("group", { name: "账号操作" });
  const impaired = actions.getByRole("button", { name: "降智", exact: true });
  await expect(card.locator("footer")).toContainText("自动检测关闭");
  await expect(impaired).toHaveCSS("height", "32px");
  await expect(impaired).toHaveCSS("width", "32px");
  await expect(card.getByText("手动标记", { exact: true })).toHaveCount(0);
  await impaired.click();
  await expect(impaired).toHaveAttribute("aria-pressed", "true");
  await expect(impaired).toHaveClass(/text-destructive/);
  await page.reload();
  await expect(impaired).toHaveAttribute("aria-pressed", "true");
  await card.getByRole("button", { name: "放大查看 动画标记测试账号 的动画" }).click();
  const preview = page.getByRole("dialog", { name: "动画预览", exact: true });
  await expect(preview).toBeVisible();
  await expect(preview.getByRole("region", { name: "动画手动标记" })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(preview).not.toBeVisible();
  await card.getByRole("button", { name: "查看动画检测详情" }).click();
  const details = page.getByRole("dialog", { name: "动画检测详情", exact: true });
  await expect(details).toBeVisible();
  await expect(details.getByRole("region", { name: "动画手动标记" })).toHaveCount(0);
  await expect(details.getByRole("region", { name: "生成原文" })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(details).not.toBeVisible();
  const normal = actions.getByRole("button", { name: "不降智", exact: true });
  await normal.click();
  await expect(normal).toHaveClass(/text-success/);
  await expect(normal).toHaveAttribute("aria-pressed", "true");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(
    await actions.evaluate((element) => {
      const bounds = element.closest("footer")!.getBoundingClientRect();
      return Array.from(element.querySelectorAll("button")).every((button) => {
        const rect = button.getBoundingClientRect();
        return rect.left >= bounds.left && rect.right <= bounds.right;
      });
    }),
  ).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("manual-verdict.png"), fullPage: true });
  await actions.getByRole("button", { name: "清除标记" }).click();
  await expect(normal).toHaveAttribute("aria-pressed", "false");
  await impaired.click();
  await expect(impaired).toHaveAttribute("aria-pressed", "true");
  await card.getByRole("button", { name: "重测 动画标记测试账号" }).click();
  await expect(impaired).toHaveAttribute("aria-pressed", "false");
  await expect(normal).toHaveAttribute("aria-pressed", "false");
});
