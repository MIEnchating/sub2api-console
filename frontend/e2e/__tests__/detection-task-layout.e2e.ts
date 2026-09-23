import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";
import type { DetectionTask, Task } from "../../src/api";

test("分组详情在宽屏显示三列，Tab 固定在顶部且只滚动当前分组结果", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const plan: DetectionTask = {
    id: "plan",
    version: 1,
    name: "分组结果",
    group_ids: ["8", "7"],
    model: "test-model",
    precheck: false,
    terminal: false,
    terminal_rounds: 1,
    automatic: false,
    schedule_type: "daily",
    daily_times: ["09:00"],
    timezone: "Asia/Shanghai",
    interval_minutes: 60,
    timeout_seconds: 120,
    last_task_id: "run",
  };
  const rows = Array.from({ length: 12 }, (_, i) => ({
    account_id: String(i + 1),
    account_name: "测试账号" + (i + 1),
    mode: "animation",
    model: "test-model",
    status: i === 0 ? "failed" : "succeeded",
    request_id: "req-" + i,
    duration_ms: 1000,
    completed_at: "2026-09-23T00:01:00Z",
    ...(i === 0
      ? { error: "HTTP 502 " + "long-error-message".repeat(150) }
      : {
          svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><rect width="640" height="400" fill="#eaf5fa"/><circle cx="320" cy="200" r="100" fill="#e9b866"/></svg>',
        }),
  }));
  const run: Task = {
    id: "run",
    skill: "sub2api-model-animation",
    operation: "managed-model-detection",
    status: "running",
    progress: 80,
    message: "正在检测其他账号",
    created_at: "2026-09-23T00:00:00Z",
    updated_at: "2026-09-23T00:01:00Z",
    result: {
      configuration: plan,
      account_ids: rows.map((r) => r.account_id),
      group_names_by_id: { "8": "备用分组", "7": "主分组" },
      group_ids_by_account: {
        "1": ["8"],
        "2": ["7", "8"],
        "3": ["8"],
        "4": ["8"],
        "5": ["7"],
        "6": ["7"],
        "7": ["7"],
        "8": ["7"],
        "9": ["8"],
        "10": ["8"],
        "11": ["8"],
        "12": ["8"],
      },
      animations: rows,
      completed: 12,
      total: 15,
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "test" },
      "/api/accounts": [],
      "/api/groups": [
        { id: "7", name: "当前已改名" },
        { id: "8", name: "备用分组" },
      ],
      "/api/model-checks/animations": [],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/detection-tasks": [plan],
      "/api/tasks/run": run,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/animation-check");
  await page.getByRole("tab", { name: "任务管理", exact: true }).click();
  await page.getByRole("button", { name: "运行详情", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测任务运行详情", exact: true });
  await expect(dialog.getByRole("region").first()).toHaveAccessibleName("分组 备用分组");
  const tabs = dialog.getByRole("tablist", { name: "检测分组" });
  await expect(tabs.getByRole("tab", { name: /备用分组/ })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(dialog.getByRole("region", { name: "分组 主分组" })).toHaveCount(0);
  await expect(dialog.getByRole("article")).toHaveCount(8);
  await expect(
    dialog
      .getByRole("region", { name: "分组 备用分组" })
      .getByRole("article", { name: "检测账号 测试账号2", exact: true }),
  ).toBeVisible();
  const previews = dialog.getByRole("group", { name: "动画预览区域" });
  await expect(previews).toHaveCount(8);
  for (const preview of await previews.all()) {
    expect(
      await preview.evaluate((el) => ({
        height: Math.round(el.getBoundingClientRect().height),
        overflow: getComputedStyle(el).overflow,
      })),
    ).toEqual({ height: 180, overflow: "hidden" });
    expect(
      await preview.evaluate((el) => {
        const card = el.closest("article")!.getBoundingClientRect();
        const box = el.getBoundingClientRect();
        return box.bottom <= card.bottom && box.right <= card.right;
      }),
    ).toBe(true);
  }
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  const wide = page.viewportSize()!.width >= 1280;
  expect(
    await dialog
      .getByRole("article")
      .first()
      .evaluate((el) => getComputedStyle(el.parentElement!).gridTemplateColumns.split(" ").length),
  ).toBe(wide ? 3 : 1);
  if (wide) expect((await dialog.boundingBox())!.width).toBeGreaterThan(1024);
  const title = dialog.getByRole("heading", { name: "检测任务运行详情" });
  const top = await title.boundingBox();
  const tabTop = await tabs.boundingBox();
  const panel = dialog.getByRole("tabpanel", { name: /备用分组/ });
  await expect
    .poll(() =>
      panel.evaluate((el) => ({
        overflowY: getComputedStyle(el).overflowY,
        scrollable: el.scrollHeight > el.clientHeight,
      })),
    )
    .toEqual({ overflowY: "auto", scrollable: true });
  await expect(dialog.getByRole("button", { name: "取消任务", exact: true })).toBeVisible();
  expect((await title.boundingBox())?.y).toBe(top?.y);
  expect((await tabs.boundingBox())?.y).toBe(tabTop?.y);
  await tabs.getByRole("tab", { name: /主分组/ }).click();
  await expect(dialog.getByRole("article")).toHaveCount(4);
  await expect(dialog.getByRole("region", { name: "分组 备用分组" })).toHaveCount(0);
  await tabs.getByRole("tab", { name: /主分组/ }).press("ArrowLeft");
  await tabs.getByRole("tab", { name: /备用分组/ }).press("Enter");
  await expect(tabs.getByRole("tab", { name: /备用分组/ })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  const zoom = dialog.getByRole("button", { name: "放大查看 测试账号2 的动画" });
  await zoom.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "动画预览", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(zoom).toBeFocused();
  await page.screenshot({ path: test.info().outputPath("grouped-details.png") });
});
