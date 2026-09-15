import { expect, test } from "@playwright/test";

import type { Task } from "../../src/api";
import { inspection } from "./fixtures/operations";
import { pageFixtures } from "./fixtures/page-shell";

const failures = Array.from({ length: 10 }, (_, index) => ({
  account_id: String(41 + index),
  changed: false,
  error: `账号 ${41 + index} 读回校验不一致，请核对管理平台状态。`,
}));
failures[0].error = `读取账号失败\nhttps://upstream.example.test/${"long-path/".repeat(50)}`;

const task: Task = {
  id: "inspection-writeback-failures",
  skill: "sub2api-auto-inspection",
  operation: "automatic-inspection",
  status: "partial",
  progress: 100,
  message: "巡检完成，但存在部分失败",
  result: {
    writeback: {
      changed: 171,
      succeeded: 371,
      failed: 10,
      results: [{ account_id: "1", changed: true }, ...failures],
    },
  },
  created_at: "2026-09-14T10:28:40Z",
  updated_at: "2026-09-14T10:30:44Z",
};

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "巡检测试" },
      "/api/tasks": [],
      [`/api/tasks/${task.id}`]: task,
      "/api/accounts": failures.map((item) => ({
        id: item.account_id,
        name: `测试账号 ${item.account_id}`,
      })),
      "/api/inspection/automation": {
        ...inspection,
        enabled: false,
        queue: [],
        last_status: "partial",
        last_task_id: task.id,
        heartbeat_history: [
          {
            checked_at: task.created_at,
            completed_at: task.updated_at,
            status: "partial",
            operations: ["routing_writeback"],
            operation_timings: [{ operation: "routing_writeback", duration_seconds: 6 }],
            task_id: task.id,
            error: "自动执行部分失败：10 项",
            skipped: false,
          },
        ],
      },
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    } else if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
    } else {
      await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    }
  });
});

test("失败心跳直接显示十个账号，长错误不横向溢出且可通过键盘滚动到最后一项", async ({ page }) => {
  await page.goto("/auto-inspection");
  const trigger = page.getByRole("button", { name: "查看心跳详情", exact: true });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "巡检心跳详情" });
  const list = dialog.getByRole("list", { name: "自动执行失败账号" });
  await expect(dialog).toHaveCSS("position", "fixed");
  await expect(list).toHaveCSS("overflow-y", "auto");
  await expect(list).toHaveCSS("max-height", "240px");
  await expect(list.getByRole("listitem")).toHaveCount(10);
  await expect(list.getByText("测试账号 41", { exact: true })).toBeVisible();
  await expect(list.getByText("ID 1", { exact: true })).toHaveCount(0);
  expect(await list.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);

  await list.focus();
  await expect(list).toBeFocused();
  await list.press("End");
  await expect(list.getByText(failures[9].error, { exact: true })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  await page.screenshot({ path: test.info().outputPath("inspection-failures.png") });
  await expect(dialog).toHaveCSS("position", "fixed");
  await page.keyboard.press("Escape");
  await expect(dialog).not.toBeVisible();
  await expect(trigger).toBeFocused();
});

test("任务明细读取失败后保留关闭和重试入口，重新读取成功后显示本轮失败账号", async ({ page }) => {
  let release!: () => void;
  const retryResponse = new Promise<void>((resolve) => {
    release = resolve;
  });
  let failed = false;
  await page.route(`**/api/tasks/${task.id}`, async (route) => {
    if (!failed) {
      failed = true;
      await route.fulfill({ status: 503, json: { detail: "巡检任务暂不可用" } });
      return;
    }
    await retryResponse;
    await route.fulfill({ json: task });
  });
  await page.goto("/auto-inspection");
  await page.getByRole("button", { name: "查看心跳详情", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "巡检心跳详情" });
  await expect(page.getByText("巡检任务暂不可用", { exact: true })).toHaveCount(1);
  await expect(dialog.getByText("巡检任务暂不可用", { exact: true })).toHaveCount(0);
  await dialog.getByRole("button", { name: "重新读取", exact: true }).click();
  await expect(dialog.getByRole("status", { name: "正在读取失败明细" })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeEnabled();
  await expect(dialog.getByRole("progressbar")).toHaveCount(0);
  release();
  await expect(
    dialog.getByRole("list", { name: "自动执行失败账号" }).getByRole("listitem"),
  ).toHaveCount(10);
  await expect(dialog.getByRole("button", { name: "重新读取", exact: true })).toHaveCount(0);
});

test("鉴权恢复触发人机验证时展示最终原因及浏览器验证指引且弹窗不横向溢出", async ({ page }) => {
  await page.route(`**/api/tasks/${task.id}`, async (route) => {
    await route.fulfill({
      json: {
        ...task,
        result: {
          ...task.result,
          upstream_sync: {
            hosts: [
              {
                host: "challenge.example.test",
                status: "auth_failed",
                reason: "refresh token 已失效",
              },
            ],
          },
          auth_recovery: {
            failed: 1,
            results: [
              {
                host: "challenge.example.test",
                success: false,
                code: "browser_challenge_required",
                reason: "登录触发浏览器人机验证",
              },
            ],
          },
        },
      },
    });
  });
  await page.goto("/auto-inspection");
  await page.getByRole("button", { name: "查看心跳详情", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "巡检心跳详情" });
  const failures = dialog.getByRole("alert");
  await expect(dialog).toHaveCSS("position", "fixed");
  await expect(failures.getByText("登录触发浏览器人机验证", { exact: true })).toBeVisible();
  await expect(failures.getByText(/恢复鉴权.*打开浏览器手动验证/)).toBeVisible();
  await expect(failures.getByText("refresh token 已失效", { exact: true })).toHaveCount(0);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
});
