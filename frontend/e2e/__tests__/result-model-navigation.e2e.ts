import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("多模型结果左右切换后预览和错误对应当前模型，卡片高度与窄屏布局稳定", async ({
  page,
  colorScheme,
}, testInfo) => {
  const longModel = "long-model-".repeat(20);
  const accounts = [
    { id: "41", name: "多模型账号", groups: [], platform: "openai" },
    { id: "42", name: "长模型账号", groups: [], platform: "openai" },
  ];
  const tasks = accounts.map((account) => ({
    id: `results-${account.id}`,
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "partial",
    progress: 100,
    message: "检测完成",
    created_at: "2026-10-13T00:00:00Z",
    updated_at: "2026-10-13T00:02:00Z",
    result: {
      account_ids: [account.id],
      targets: [
        { account_id: account.id, model: "gpt-6-astra", models: ["gpt-6-astra", "gpt-6.1-sol"] },
      ],
      animations: [
        {
          account_id: account.id,
          account_name: account.name,
          model: "gpt-6-astra",
          request_id: `success-${account.id}`,
          mode: "animation",
          status: "succeeded",
          svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><rect width="640" height="400" fill="#10283f"/><circle cx="320" cy="200" r="85" fill="#38bdf8"/></svg>',
          duration_ms: 45000,
          completed_at: "2026-10-13T00:01:00Z",
        },
        {
          account_id: account.id,
          account_name: account.name,
          model: account.id === "42" ? longModel : "gpt-6.1-sol",
          request_id: `failed-${account.id}`,
          mode: "animation",
          status: "failed",
          error: "这个模型暂时不可用，请稍后重测",
          duration_ms: 3000,
          completed_at: "2026-10-13T00:00:30Z",
        },
      ],
    },
  }));
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "dark"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "结果布局测试" },
      "/api/accounts": accounts,
      "/api/model-checks/animations": tasks,
      "/api/model-checks/animation-schedules": [],
      ...Object.fromEntries(tasks.map((task) => [`/api/tasks/${task.id}`, task])),
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  const card = page.getByRole("article", { name: "账号 多模型账号", exact: true });
  const navigation = page.getByRole("group", { name: "结果模型", exact: true });
  await expect(navigation).toContainText("gpt-6-astra");
  await expect(navigation).toContainText("1 / 3");
  await expect(navigation.getByRole("button", { name: "上一个模型结果" })).toBeDisabled();
  await expect(card.getByRole("img")).toBeVisible();
  const height = (await card.boundingBox())!.height;
  await navigation.getByRole("button", { name: "下一个模型结果" }).click();
  await expect(navigation).toContainText("gpt-6.1-sol");
  await expect(navigation).toContainText("2 / 3");
  await expect(card.getByText("这个模型暂时不可用，请稍后重测")).toBeVisible();
  await expect(card.getByRole("img")).toHaveCount(0);
  expect((await card.boundingBox())!.height).toBe(height);
  await expect(navigation.getByRole("button", { name: "下一个模型结果" })).toBeEnabled();
  await navigation.focus();
  await page.keyboard.press("ArrowLeft");
  await expect(card.getByRole("img")).toBeVisible();
  await expect(navigation).toContainText("gpt-6-astra");
  const longCard = page.getByRole("article", { name: "账号 长模型账号", exact: true });
  await navigation.getByRole("button", { name: "下一个模型结果" }).click();
  await expect(longCard.getByText("该模型暂无检测结果")).toBeVisible();
  await expect(longCard.getByRole("button", { name: "下一个模型结果" })).toHaveCount(0);
  await navigation.getByRole("button", { name: "下一个模型结果" }).click();
  await expect(navigation.getByTitle(longModel, { exact: true })).toBeVisible();
  await expect(card.getByText("该模型暂无检测结果")).toBeVisible();
  expect(await longCard.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await card.scrollIntoViewIfNeeded();
  await page.screenshot({ path: testInfo.outputPath("result-navigation.png") });
});
