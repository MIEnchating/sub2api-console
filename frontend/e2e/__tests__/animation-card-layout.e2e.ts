import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";
import type { AnimationResult, AnimationSchedule, Task } from "../../src/api";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

const accounts = [1, 2, 3, 4].map((id) => ({
  ...account,
  id: String(id),
  platform: "openai",
  name: `检测账号 ${id} · ${"长名称".repeat(8)}`,
}));
const answer = "无法提供具体的知识截止日期。".repeat(30);
const error = "上游暂时无法提供服务，请稍后重试。".repeat(30);
const svg =
  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><rect width="640" height="400" fill="#eef2ff"/><circle cx="320" cy="200" r="60" fill="#4338ca"><animate attributeName="r" values="60;80;60" dur="2s" repeatCount="indefinite"/></circle></svg>';

async function openAnimationCards(page: Page): Promise<{ schedules: AnimationSchedule[] }> {
  const state = { schedules: [] as AnimationSchedule[] };
  const rows: AnimationResult[] = accounts.slice(0, 3).flatMap((item) => {
    const base = {
      account_id: item.id,
      account_name: item.name,
      model: "gpt-6-astra",
      status: "succeeded" as const,
      request_id: `check-${item.id}`,
      duration_ms: 1234,
      completed_at: "2026-09-15T00:01:00Z",
    };
    const precheck: AnimationResult = {
      ...base,
      mode: "precheck",
      precheck: {
        verdict: "passed",
        profile_version: "astra-v1",
        questions: [
          { id: "candy", verdict: "passed", answer: "21", request_id: `candy-${item.id}` },
          {
            id: "knowledge-cutoff",
            verdict: "passed",
            answer,
            request_id: `cutoff-${item.id}`,
          },
        ],
      },
    };
    if (item.id === "3") return [precheck];
    if (item.id === "2") {
      precheck.precheck!.questions = precheck.precheck!.questions.slice(0, 1);
      return [precheck, { ...base, mode: "animation", status: "failed", error }];
    }
    return [precheck, { ...base, mode: "animation", svg }];
  });
  const task: Task = {
    id: "combined-card",
    skill: "sub2api-model-animation",
    operation: "account-model-combined",
    status: "succeeded",
    progress: 100,
    message: "检测完成",
    result: { mode: "both", account_ids: accounts.map((item) => item.id), animations: rows },
    created_at: "2026-09-15T00:00:00Z",
    updated_at: "2026-09-15T00:01:00Z",
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/animation-schedules/1" && route.request().method() === "PUT") {
      state.schedules = [{ ...route.request().postDataJSON(), version: 1 }];
      await route.fulfill({ json: state.schedules });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "卡片布局测试" },
      "/api/overview": {
        database_available: true,
        account_count: accounts.length,
        group_count: 0,
        open_alerts: 0,
        recent_runs: 0,
        last_activity: null,
        mode: "完全模式",
      },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": [],
      "/api/accounts": accounts,
      "/api/model-checks/capabilities": { claude_standards: [], sol_models: [] },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/animation-schedules": state.schedules,
      "/api/model-checks/animations": [{ ...task, result: {} }],
      "/api/tasks/combined-card": task,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/model-check");
  await page.getByRole("tab", { name: "动画检测", exact: true }).click();
  await expect(page.getByRole("article")).toHaveCount(accounts.length);
  await expect(page.getByRole("button", { name: "查看前置检测详情" })).toHaveCount(3);
  return state;
}

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

test("单题双题和未检测混排时底部按内容收紧，预览保持对齐", async ({ page }) => {
  await openAnimationCards(page);
  const cards = page.getByRole("article");
  const image = cards.first().getByRole("img");
  await expect(image).toHaveJSProperty("complete", true);
  expect(await image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBeGreaterThan(
    0,
  );
  const firstBox = (await cards.first().boundingBox())!;
  expect(firstBox.height).toBeLessThanOrEqual(450);
  for (const card of await cards.all()) {
    const box = (await card.boundingBox())!;
    const summary = card.getByRole("region", { name: "前置检测结果", exact: true });
    const lastRow = summary.getByRole("listitem").last();
    const content = (await lastRow.count()) > 0 ? lastRow : summary;
    const contentBox = (await content.boundingBox())!;
    const footerBox = (await card.locator("footer").boundingBox())!;
    expect(footerBox.y - contentBox.y - contentBox.height).toBeLessThanOrEqual(8);
    expect(footerBox.y).toBeGreaterThanOrEqual(contentBox.y + contentBox.height);
    const preview = card.getByRole("group", { name: "动画预览区域", exact: true });
    await expect(preview).toHaveCSS("height", "180px");
    expect((await preview.boundingBox())!.y - box.y).toBe(
      (await cards.first().getByRole("group", { name: "动画预览区域", exact: true }).boundingBox())!
        .y - firstBox.y,
    );
    await expect(card.locator("details")).toHaveCount(0);
    await expect(card.getByText(answer, { exact: true })).toHaveCount(0);
  }
  const singleQuestionBox = (await cards.nth(1).boundingBox())!;
  const uncheckedBox = (await cards.nth(3).boundingBox())!;
  expect(singleQuestionBox.height).toBeLessThan(firstBox.height);
  expect(uncheckedBox.height).toBeLessThan(singleQuestionBox.height);
  const region = page.getByRole("region", { name: "动画账号卡片", exact: true });
  expect(await region.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (page.viewportSize()!.width >= 1440) {
    await expect(cards.first()).toBeInViewport({ ratio: 1 });
    const settings = page.getByRole("group", { name: "动画检测设置", exact: true });
    expect((await settings.boundingBox())!.height).toBeLessThanOrEqual(144);
    expect(
      (await page.getByRole("combobox", { name: "检测模型", exact: true }).boundingBox())!.width,
    ).toBeLessThanOrEqual(420);
  }
  await page.screenshot({ path: test.info().outputPath("card-preview.png") });
  if (page.viewportSize()!.width < 768) {
    await cards.first().scrollIntoViewIfNeeded();
    await expect(cards.first().getByRole("checkbox")).toBeInViewport({ ratio: 1 });
    await expect(cards.first().locator("footer")).toBeInViewport({ ratio: 1 });
    await page.screenshot({ path: test.info().outputPath("card-scrolled.png") });
  }
});

test("卡片详情可用键盘打开，长回答与完整错误只在详情内展示且关闭后焦点返回", async ({ page }) => {
  await openAnimationCards(page);
  const first = page.getByRole("article").first();
  const before = (await first.boundingBox())!;
  const precheckButton = first.getByRole("button", { name: "查看前置检测详情" });
  await precheckButton.focus();
  await page.keyboard.press("Enter");
  const precheck = page.getByRole("dialog", { name: /前置检测详情/ });
  await expect(precheck).toBeVisible();
  await expect(precheck.getByText(answer, { exact: true })).toHaveCount(1);
  expect(await precheck.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("precheck-details.png") });
  await page.keyboard.press("Escape");
  await expect(precheck).not.toBeVisible();
  await expect(precheckButton).toBeFocused();
  expect((await first.boundingBox())!.height).toBe(before.height);
  const failed = page.getByRole("article").nth(1);
  const animationButton = failed.getByRole("button", { name: "查看动画检测详情" });
  await animationButton.focus();
  await page.keyboard.press("Enter");
  const animation = page.getByRole("dialog", { name: /动画检测详情/ });
  await expect(animation).toBeVisible();
  await expect(animation.getByText(error, { exact: true })).toHaveCount(1);
  await expect(animation).toContainText("check-2");
  expect(await animation.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("animation-details.png") });
  await page.keyboard.press("Escape");
  await expect(animation).not.toBeVisible();
  await expect(animationButton).toBeFocused();
});

test("卡片自动检测可同时保存前置与动画，并在重新打开后保留所选题目", async ({ page }) => {
  const state = await openAnimationCards(page);
  const first = page.getByRole("article").first();
  await first.getByRole("button", { name: "自动检测设置" }).click();
  const dialog = page.getByRole("dialog", {
    name: `自动检测设置 · ${accounts[0].name}`,
    exact: true,
  });
  await dialog.getByRole("checkbox", { name: "前置检测", exact: true }).check();
  await expect(dialog.getByRole("checkbox", { name: "动画检测", exact: true })).toBeChecked();
  await dialog.getByRole("checkbox", { name: "开启自动检测" }).check();
  await dialog.getByRole("textbox", { name: "检测模型", exact: true }).fill("gpt-6-astra");
  await dialog.getByRole("button", { name: "选择前置检测题目" }).click();
  const questions = page.getByRole("dialog", { name: "前置检测题目", exact: true });
  await questions.getByRole("checkbox", { name: "知识截止日期", exact: true }).uncheck();
  await page.keyboard.press("Escape");
  await page.screenshot({ path: test.info().outputPath("combined-schedule.png") });
  await dialog.getByRole("button", { name: "保存设置", exact: true }).click();
  const confirm = page.getByRole("dialog", { name: "确认开启自动检测", exact: true });
  await expect(confirm).toContainText("随后生成一次动画");
  await confirm.getByRole("button", { name: "确认保存并开启", exact: true }).click();
  await expect.poll(() => state.schedules[0]?.mode).toBe("both");
  expect(state.schedules[0].precheck_questions).toEqual(["candy"]);
  await first.getByRole("button", { name: "自动检测设置" }).click();
  await expect(dialog.getByRole("checkbox", { name: "前置检测", exact: true })).toBeChecked();
  await expect(dialog.getByRole("checkbox", { name: "动画检测", exact: true })).toBeChecked();
  await dialog.getByRole("button", { name: "选择前置检测题目" }).click();
  await expect(questions.getByRole("checkbox", { name: "糖果题", exact: true })).toBeChecked();
  await expect(
    questions.getByRole("checkbox", { name: "知识截止日期", exact: true }),
  ).not.toBeChecked();
});
