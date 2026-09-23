import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";
import type { AnimationRequest, AnimationResult, AnimationSchedule, Task } from "../../src/api";
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
  await page.goto("/animation-check");
  await expect(page.getByRole("article")).toHaveCount(accounts.length);
  await expect(page.getByRole("button", { name: "查看动画检测详情" })).toHaveCount(2);
  return state;
}

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
});

test("动画页只显示动画结果，卡片和预览保持对齐", async ({ page }) => {
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
    await expect(card.getByRole("region", { name: "前置检测结果" })).toHaveCount(0);
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
  expect(singleQuestionBox.height).toBe(firstBox.height);
  expect(uncheckedBox.height).toBe(singleQuestionBox.height);
  const region = page.getByRole("region", { name: "动画账号卡片", exact: true });
  for (const card of await cards.all()) {
    for (const button of await card.getByRole("button").all()) {
      const buttonBox = (await button.boundingBox())!;
      const cardBox = (await card.boundingBox())!;
      expect(buttonBox.x).toBeGreaterThanOrEqual(cardBox.x);
      expect(buttonBox.x + buttonBox.width).toBeLessThanOrEqual(cardBox.x + cardBox.width);
    }
  }
  await expect(page.getByRole("group", { name: "前置检测操作", exact: true })).toHaveCount(0);
  expect(await region.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  if (page.viewportSize()!.width >= 1440) {
    await expect(cards.first()).toBeInViewport({ ratio: 1 });
    const settings = page.getByRole("group", { name: "动画检测设置", exact: true });
    expect((await settings.boundingBox())!.height).toBeLessThanOrEqual(168);
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

test("成功卡片重测沿用原模型，启动与完成时卡片高度稳定且禁止重复提交", async ({ page }) => {
  await openAnimationCards(page);
  const requests: AnimationRequest[] = [];
  let current: Task = {
    id: "retest-card",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "running",
    progress: 0,
    message: "正在生成动画",
    created_at: "2026-09-22T00:00:00Z",
    updated_at: "2026-09-22T00:00:00Z",
    result: { account_ids: ["1"], animations: [] },
  };
  await page.route("**/api/model-checks/animations", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    requests.push(route.request().postDataJSON() as AnimationRequest);
    await route.fulfill({ json: current });
  });
  await page.route("**/api/tasks/retest-card", (route) => route.fulfill({ json: current }));
  await page.getByRole("combobox", { name: "检测模型", exact: true }).fill("different-model");
  await page.keyboard.press("Escape");
  const card = page.getByRole("article").first();
  const before = (await card.boundingBox())!;
  const retest = card.getByRole("button", { name: `重测 ${accounts[0].name}`, exact: true });
  await retest.focus();
  await page.keyboard.press("Enter");
  await expect(card.getByRole("status", { name: "生成中，等待动画结果" })).toBeVisible();
  await expect(retest).toBeDisabled();
  expect(requests).toEqual([
    { targets: [{ account_id: "1", model: "gpt-6-astra" }], timeout_seconds: 120 },
  ]);
  expect((await card.boundingBox())!.height).toBe(before.height);
  await expect(
    page
      .getByRole("article")
      .nth(1)
      .getByRole("button", { name: /^重测 / }),
  ).toBeEnabled();
  current = {
    ...current,
    status: "succeeded",
    progress: 100,
    result: {
      account_ids: ["1"],
      animations: [
        {
          account_id: "1",
          account_name: accounts[0].name,
          model: "gpt-6-astra",
          status: "succeeded",
          svg,
          request_id: "retest-result",
          duration_ms: 2500,
          completed_at: "2026-09-22T00:01:00Z",
        },
      ],
    },
  };
  await expect(retest).toBeEnabled();
  await expect(card.getByRole("img")).toBeVisible();
  await expect(card.getByText("2.5 秒", { exact: true })).toBeVisible();
  expect((await card.boundingBox())!.height).toBe(before.height);
  expect(requests).toHaveLength(1);
});

test("卡片详情可用键盘打开，长回答与完整错误只在详情内展示且关闭后焦点返回", async ({ page }) => {
  await openAnimationCards(page);
  await page.getByRole("tab", { name: "前置检测", exact: true }).click();
  await expect(page.getByRole("button", { name: "查看前置检测详情" })).toHaveCount(3);
  await expect(page.getByRole("group", { name: "动画预览区域" })).toHaveCount(0);
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
  await page.getByRole("tab", { name: "账号检测", exact: true }).click();
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

test("前置检测自动计划独立保存，每天指定时间和题目在重开后保留", async ({ page }) => {
  const state = await openAnimationCards(page);
  await page.getByRole("tab", { name: "前置检测", exact: true }).click();
  const first = page.getByRole("article").first();
  await first.getByRole("button", { name: "自动检测设置" }).click();
  const dialog = page.getByRole("dialog", {
    name: `自动前置检测设置 · ${accounts[0].name}`,
    exact: true,
  });
  await dialog.getByRole("radio", { name: "每天定时" }).check();
  await dialog.getByLabel("每天检测时间（北京时间）").fill("09:30");
  await dialog.getByRole("checkbox", { name: "开启自动检测" }).check();
  await dialog.getByRole("textbox", { name: "检测模型", exact: true }).fill("gpt-6-astra");
  await dialog.getByRole("button", { name: "选择前置检测题目" }).click();
  const questions = page.getByRole("dialog", { name: "前置检测题目", exact: true });
  await expect(questions.getByRole("checkbox", { name: "糖果题", exact: true })).toBeChecked();
  await page.keyboard.press("Escape");
  await page.screenshot({ path: test.info().outputPath("combined-schedule.png") });
  await dialog.getByRole("button", { name: "保存设置", exact: true }).click();
  const confirm = page.getByRole("dialog", { name: "确认开启自动检测", exact: true });
  await expect(confirm).toContainText("每天 09:30（北京时间）");
  await confirm.getByRole("button", { name: "确认保存并开启", exact: true }).click();
  await expect.poll(() => state.schedules[0]?.mode).toBe("precheck");
  expect(state.schedules[0].precheck_questions).toEqual(["candy"]);
  await first.getByRole("button", { name: "自动检测设置" }).click();
  await expect(dialog.getByLabel("每天检测时间（北京时间）")).toHaveValue("09:30");
  await dialog.getByRole("button", { name: "选择前置检测题目" }).click();
  await expect(questions.getByRole("checkbox", { name: "糖果题", exact: true })).toBeChecked();
  await expect(questions.getByRole("checkbox", { name: "知识截止日期", exact: true })).toHaveCount(
    0,
  );
});
