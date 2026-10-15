import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

test("缺失字段与用量变化同时可见且长型号只在证据区域内滚动", async ({ page }, testInfo) => {
  const task = {
    id: "diagnostics",
    skill: "sub2api-model-check",
    status: "succeeded",
    progress: 100,
    message: "完成",
    result: {
      tests: [
        {
          checker: "verification",
          account_id: "41",
          claimed_model: "new-model",
          capability: { status: "passed" },
          reference: { status: "not_run", summary: "未选择官方参考" },
          fingerprint: {
            status: "unstable",
            summary: "用量发生变化，其他缺失字段见明细",
            samples: [
              {
                id: "baseline",
                label: "基准文本",
                status: "unavailable",
                first: 59,
                repeat: 59,
                detail: "首次未返回型号；复测未返回型号",
                protocol: "openai_responses",
                repeat_protocol: "openai_responses",
              },
              {
                id: "english",
                label: "英文文本",
                status: "unstable",
                first: 436,
                repeat: 79,
                detail: "首次未返回型号；复测未返回型号；输入用量变化：436 → 79 Token",
                protocol: "openai_responses",
                repeat_protocol: "openai_responses",
              },
              {
                id: "code",
                label: "代码文本",
                status: "unavailable",
                first: 4290,
                repeat: 4290,
                model: "long-model-".repeat(30),
                repeat_model: "long-model-".repeat(30),
                detail:
                  "本项重复采样一致，但基准文本不可用，无法计算差分。基准原因：首次未返回型号；复测未返回型号",
              },
            ],
          },
          total: 17,
          passed: 7,
          capability_total: 7,
        },
      ],
    },
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
      "/api/accounts": [{ ...account, id: "41", platform: "openai" }],
      "/api/accounts/41/models": { models: ["new-model"] },
      "/api/model-checks": task,
      "/api/tasks/diagnostics": task,
    };
    await route.fulfill({ json: fixtures[path] ?? [] });
  });
  await page.goto("/model-check?account_id=41");
  await page.getByRole("checkbox", { name: "选择模型 new-model" }).check();
  await page.getByRole("button", { name: /开始检测/ }).click();
  await page.getByRole("button", { name: "查看检测证据" }).click();
  const dialog = page.getByRole("dialog", { name: "检测证据", exact: true });
  await dialog.getByRole("button", { name: "来源与指纹" }).click();
  const change = dialog.getByText(/首次未返回型号；复测未返回型号；输入用量变化：436 → 79 Token/);
  await change.scrollIntoViewIfNeeded();
  await expect(change).toBeVisible();
  const baseline = dialog.getByText(/本项重复采样一致，但基准文本不可用/);
  await baseline.scrollIntoViewIfNeeded();
  await expect(baseline).toBeVisible();
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeInViewport({
    ratio: 1,
  });
  expect(await dialog.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
  await page.screenshot({ path: testInfo.outputPath("fingerprint-diagnostics.png") });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "查看检测证据" })).toBeFocused();
});
