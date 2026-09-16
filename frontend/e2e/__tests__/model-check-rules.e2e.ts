import { expect, test, type Page } from "@playwright/test";
import { waitForLayoutAnimations } from "../layout-motion";
import { pageFixtures } from "./fixtures/page-shell";

import { configurationFixture, mockReadRoutes } from "./fixtures/model-check-configuration";

async function selectRule(page: Page, model: string): Promise<void> {
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  if (page.viewportSize()!.width >= 768) {
    const button = dialog.getByRole("button", { name: model, exact: true });
    await button.click();
    await expect(button).toHaveAttribute("aria-pressed", "true");
    await expect(dialog.getByRole("heading", { name: model, exact: true })).toBeVisible();
  } else {
    await dialog.getByRole("combobox", { name: "查看检测规则" }).click();
    await page.getByRole("option", { name: model, exact: true }).click();
    await expect(dialog.getByRole("combobox", { name: "查看检测规则" })).toContainText(model);
  }
}

test("长题目滚动和翻页后，搜索与分页保持固定且新页从顶部开始", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const configuration = configurationFixture();
  const profile = configuration.active.payload.claude_profiles["claude-opus-5"];
  profile.probes = profile.probes.map((probe) => ({
    ...probe,
    question: `${probe.question}\n${"分析选项中的不同回答特征。".repeat(30)}`,
  }));
  await mockReadRoutes(page, configuration);
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await dialog.getByRole("tab", { name: "检测题目", exact: true }).click();
  const pager = dialog.getByRole("navigation", { name: "表格分页" });
  const search = dialog.getByRole("textbox", { name: "搜索检测题目" });
  await expect(pager).toBeInViewport({ ratio: 1 });
  await waitForLayoutAnimations(dialog);
  const pagerPosition = await pager.boundingBox();
  const searchPosition = await search.boundingBox();
  const list = dialog.getByRole("list", { name: "检测题目列表" });
  await list.focus();
  await page.keyboard.press("Control+End");
  await expect
    .poll(() =>
      list.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop),
    )
    .toBeLessThanOrEqual(1);
  await expect(pager).toBeInViewport({ ratio: 1 });
  expect(await pager.boundingBox()).toEqual(pagerPosition);
  expect(await search.boundingBox()).toEqual(searchPosition);
  await pager.getByRole("button", { name: "转到下一页" }).click();
  await expect(list.getByText("claude-6", { exact: true })).toBeInViewport({ ratio: 1 });
  await expect.poll(() => list.evaluate((element) => element.scrollTop)).toBe(0);
  expect(await pager.boundingBox()).toEqual(pagerPosition);
  await expect(pager.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  await page.screenshot({ path: testInfo.outputPath("questions-fixed-pagination.png") });
  await search.fill("没有匹配的题目");
  await expect(dialog.getByText("暂无匹配题目")).toBeInViewport({ ratio: 1 });
  expect(await pager.boundingBox()).toEqual(pagerPosition);
  await expect(pager.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  await expect(pager.getByRole("button", { name: "转到下一页" })).toBeDisabled();
});

test("切换高级设置后返回，保留选中的模型、题目搜索和页码", async ({ page }) => {
  const configuration = configurationFixture();
  const profile = configuration.active.payload.claude_profiles["claude-opus-5"];
  configuration.active.payload.claude_profiles["claude-sonnet-5"] = {
    ...profile,
    identity_group: ["claude-sonnet-5"],
  };
  await mockReadRoutes(page, configuration);
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await selectRule(page, "claude-sonnet-5");
  await dialog.getByRole("tab", { name: "检测题目", exact: true }).click();
  await dialog.getByRole("textbox", { name: "搜索检测题目" }).fill("claude");
  await dialog.getByRole("button", { name: "转到下一页" }).click();
  await expect(dialog.getByText("检测题目 6", { exact: true })).toBeVisible();
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  await expect(dialog.getByRole("textbox", { name: "搜索检测题目" })).toHaveCount(0);
  await dialog.getByRole("tab", { name: "规则与题目" }).click();
  await expect(dialog.getByRole("tab", { name: "检测题目", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(dialog.getByRole("textbox", { name: "搜索检测题目" })).toHaveValue("claude");
  await expect(dialog.getByText("检测题目 6", { exact: true })).toBeInViewport({ ratio: 1 });
  if (page.viewportSize()!.width >= 768) {
    await expect(
      dialog.getByRole("button", { name: "claude-sonnet-5", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
  } else {
    await expect(dialog.getByRole("combobox", { name: "查看检测规则" })).toContainText(
      "claude-sonnet-5",
    );
  }
});

test("大型题库使用固定尺寸编辑区，切换视图后保留未保存的 JSON 与说明", async ({ page }) => {
  const configuration = configurationFixture();
  const source = configuration.active.payload.claude_profiles["claude-opus-5"];
  configuration.active.payload.claude_profiles = Object.fromEntries(
    Array.from({ length: 10 }, (_, model) => [
      `claude-test-${model}`,
      {
        ...source,
        probes: Array.from({ length: 100 }, (_, index) => ({
          ...source.probes[0],
          id: `probe-${index}`,
          question: `题目 ${index}：${"请分析这些选项的回答特征。".repeat(10)}`,
        })),
      },
    ]),
  );
  await mockReadRoutes(page, configuration);
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  const editor = dialog.getByRole("textbox", { name: "规则与题库 JSON" });
  await expect(editor).toHaveAttribute("contenteditable", "true");
  const editorFrame = dialog.locator('[data-slot="json-editor"]');
  const height = await editorFrame.evaluate((element) => element.clientHeight);
  const editedPayload = JSON.stringify(configuration.active.payload, null, 2) + "\n";
  await editor.press("Control+End");
  await editor.press("Enter");
  await dialog.getByRole("textbox", { name: "版本说明" }).fill("未保存的版本说明");
  await dialog.getByRole("tab", { name: "规则与题目" }).click();
  await expect(editor).toHaveCount(0);
  const inactivePanel = dialog.locator('[role="tabpanel"]').filter({
    has: page.locator("#model-check-profile-json"),
  });
  await expect(inactivePanel).toHaveAttribute("inert", "");
  await expect(inactivePanel).toHaveCSS("visibility", "hidden");
  expect(await editorFrame.evaluate((element) => element.clientHeight)).toBe(height);
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await editorFrame.getByRole("button", { name: "复制内容" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(editedPayload);
  await expect(dialog.getByRole("textbox", { name: "版本说明" })).toHaveValue("未保存的版本说明");
  expect(await editorFrame.evaluate((element) => element.clientHeight)).toBe(height);
  expect(
    await editorFrame
      .locator(".cm-scroller")
      .evaluate((element) => element.scrollHeight > element.clientHeight),
  ).toBe(true);
  await expect(dialog.getByRole("button", { name: "保存草稿" })).toBeInViewport({ ratio: 1 });
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
});

test("默认按系列展示规则，GPT 题目和阈值可读，搜索分页与键盘切换在窄屏可用", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await mockReadRoutes(page, configurationFixture());
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await expect(dialog.getByRole("tab", { name: "规则与题目" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(dialog.getByRole("region", { name: "支持的模型" })).toContainText("gpt-5.6-sol");
  await expect(dialog.getByRole("region", { name: "支持的模型" })).toContainText("gpt-5.6-luna");
  await expect(dialog.getByRole("region", { name: "支持的模型" })).toContainText("gpt-5.6-terra");
  await expect(dialog.getByRole("textbox", { name: "规则与题库 JSON" })).toHaveCount(0);
  await expect(dialog.getByRole("tab", { name: "判定标准", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(dialog.getByRole("table", { name: "模型判定阈值" })).toBeInViewport({ ratio: 1 });
  await expect(dialog.getByRole("textbox", { name: "搜索检测题目" })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath("rules-overview.png") });
  await dialog.getByRole("tab", { name: "检测题目", exact: true }).click();
  await dialog.getByRole("button", { name: "转到下一页" }).click();
  await expect(dialog.getByText("检测题目 6", { exact: true })).toBeVisible();
  await expect(dialog.getByText("检测题目 1", { exact: true })).toHaveCount(0);
  const search = dialog.getByRole("textbox", { name: "搜索检测题目" });
  await search.fill("不存在的题目");
  await expect(dialog.getByText("暂无匹配题目")).toBeVisible();
  await search.fill("检测题目 1");
  await expect(dialog.getByText("检测题目 1", { exact: true })).toBeVisible();
  await selectRule(page, "gpt-5.6-sol");
  await expect(dialog.getByText("≥ 70%", { exact: true })).toBeVisible();
  await expect(dialog.getByText("≥ 75%", { exact: true })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("gpt-rules.png") });
  await dialog.getByRole("tab", { name: "检测题目", exact: true }).click();
  await expect(search).toHaveValue("");
  await expect(dialog.getByText("水的沸点是多少？")).toBeVisible();
  await expect(dialog.getByText("选择距离")).toBeVisible();
  await expect(dialog.getByText(/容差：5%（相对）/)).toBeVisible();
  await dialog.getByText("评分权重", { exact: true }).first().click();
  await expect(dialog.getByText("特征值 100", { exact: true })).toBeVisible();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("gpt-questions.png") });
  for (const model of ["gpt-5.6-luna", "gpt-5.6-terra"]) {
    await selectRule(page, model);
    await expect(dialog.getByText(`${model} 在另外两个候选中的占比`, { exact: true })).toHaveCount(
      1,
    );
    await page.screenshot({ path: testInfo.outputPath(`${model}-rules.png`) });
    await dialog.getByRole("tab", { name: "检测题目", exact: true }).click();
    await dialog.getByText("评分权重", { exact: true }).first().click();
    await expect(
      dialog.getByText(`${model}（当前检测模型）`, { exact: true }).first(),
    ).toBeVisible();
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
  }
  await dialog.getByRole("tab", { name: "规则与题目" }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(dialog.getByRole("tab", { name: "高级设置" })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(dialog.getByRole("tab", { name: "高级设置" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(dialog.getByRole("textbox", { name: "规则与题库 JSON" })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "发布生效" })).toBeDisabled();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("button", { name: "检测规则与题库", exact: true })).toBeFocused();
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  await expect(dialog.getByRole("tab", { name: "规则与题目" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
});

test("多规则和超长模型名、题目及权重完整可读，不撑宽弹窗", async ({
  page,
  colorScheme,
}, testInfo) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  const configuration = configurationFixture();
  const source = configuration.active.payload.claude_profiles["claude-opus-5"];
  const model = "claude-" + "long-model-name-".repeat(12);
  const question = "long-question-content-".repeat(30);
  configuration.active.payload.claude_profiles = Object.fromEntries(
    [
      model,
      "claude-haiku-4.5",
      "claude-opus-4.5",
      "claude-opus-4.6",
      "claude-opus-4.7",
      "claude-opus-4.8",
      "claude-opus-5",
      "claude-sonnet-4",
      "claude-sonnet-4.5",
      "claude-sonnet-5",
    ].map((name) => [name, source]),
  );
  configuration.active.payload.claude_profiles[model] = {
    ...source,
    candidate_models: [model, "claude-sonnet-5"],
    probes: [{ ...source.probes[0], question }],
  };
  await mockReadRoutes(page, configuration);
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await selectRule(page, model);
  await dialog.getByRole("tab", { name: "检测题目", exact: true }).click();
  await dialog.getByText(question, { exact: true }).scrollIntoViewIfNeeded();
  await expect(dialog.getByText(question, { exact: true })).toBeVisible();
  await dialog.getByText("评分权重", { exact: true }).click();
  await expect(dialog.getByRole("term").filter({ hasText: model }).first()).toBeVisible();
  const areas = dialog.getByRole("region");
  for (const area of await areas.all())
    expect(await area.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("rules-long-content.png") });
});

test("保存草稿不改变生效规则，预览草稿并确认发布后才更新支持模型", async ({ page }) => {
  let configuration = configurationFixture();
  let saveCount = 0;
  let publishCount = 0;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/model-checks/configuration/draft") {
      const body = route.request().postDataJSON();
      expect(body.expected_fingerprint).toBe("active-fingerprint");
      expect(body.note).toBe("新规则");
      configuration = {
        ...configuration,
        draft: {
          ...configuration.active,
          id: "rules-draft",
          status: "draft",
          fingerprint: "draft-fingerprint",
          payload: body.payload,
          note: body.note,
        },
      };
      saveCount += 1;
      await route.fulfill({ json: configuration });
      return;
    }
    if (path === "/api/model-checks/configuration/publish") {
      expect(route.request().postDataJSON()).toEqual({ expected_fingerprint: "draft-fingerprint" });
      configuration = {
        ...configuration,
        active: { ...configuration.draft!, status: "published" },
        draft: null,
      };
      publishCount += 1;
      await route.fulfill({ json: configuration });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "规则测试" },
      "/api/accounts": [],
      "/api/model-checks/capabilities": {
        claude_standards: ["claude-opus-5"],
        sol_models: configuration.active.payload.sol_profile.candidate_models,
      },
      "/api/model-checks/account-statuses": [],
      "/api/model-checks/configuration": configuration,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  const payload = structuredClone(configuration.active.payload);
  payload.sol_profile.candidate_models[0] = "gpt-custom-sol";
  await dialog
    .getByRole("textbox", { name: "规则与题库 JSON" })
    .fill(JSON.stringify(payload, null, 2));
  await dialog.getByRole("textbox", { name: "版本说明" }).fill("新规则");
  await dialog.getByRole("button", { name: "保存草稿" }).click();
  await expect(dialog.getByRole("button", { name: "发布生效" })).toBeEnabled();
  expect(saveCount).toBe(1);
  await dialog.getByRole("tab", { name: "规则与题目" }).click();
  await expect(dialog.getByRole("region", { name: "支持的模型" })).toContainText("gpt-5.6-sol");
  await expect(dialog.getByRole("region", { name: "支持的模型" })).not.toContainText(
    "gpt-custom-sol",
  );
  await dialog.getByRole("combobox", { name: "查看规则版本" }).click();
  await page.getByRole("option", { name: "草稿（未生效）" }).click();
  await expect(dialog.getByRole("region", { name: "支持的模型" })).toContainText("gpt-custom-sol");
  await expect(dialog.getByText(/草稿预览，尚未生效/)).toBeVisible();
  await dialog.getByRole("tab", { name: "高级设置" }).click();
  await dialog.getByRole("textbox", { name: "版本说明" }).fill("未保存的修改");
  await expect(dialog.getByRole("button", { name: "发布生效" })).toBeDisabled();
  await dialog.getByRole("button", { name: "恢复已保存内容" }).click();
  await dialog.getByRole("button", { name: "发布生效" }).click();
  expect(publishCount).toBe(0);
  await page
    .getByRole("dialog", { name: "发布检测规则", exact: true })
    .getByRole("button", { name: "确认发布" })
    .click();
  await expect(dialog.getByRole("button", { name: "发布生效" })).toBeDisabled();
  expect(publishCount).toBe(1);
  await dialog.getByRole("tab", { name: "规则与题目" }).click();
  await expect(dialog.getByRole("region", { name: "支持的模型" })).toContainText("gpt-custom-sol");
  await expect(dialog.getByText(/当前生效 · rules-draft/)).toBeVisible();
});
