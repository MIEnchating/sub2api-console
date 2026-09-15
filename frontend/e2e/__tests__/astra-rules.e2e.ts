import { expect, test } from "@playwright/test";
import { configurationFixture, mockReadRoutes } from "./fixtures/model-check-configuration";

test("Astra 内置题库可在桌面和移动端选择并查看 low、mid 预期回答", async ({ page }, testInfo) => {
  const configuration = configurationFixture();
  configuration.builtin_astra_profile = {
    model: "gpt-6-astra",
    version: "astra-v1",
    questions: [
      {
        id: "candy",
        effort: "low",
        expected: "21",
        question:
          "袋子里有三种口味的糖——苹果、桃子、西瓜。每种口味又有两种形状，圆形和五角星。摸糖的时候，形状靠手感就能分辨。\n苹果 桃子 西瓜\n圆形 7 9 8\n五角星 7 6 4\n最少取多少颗，能保证手里同时有一组「圆形苹果 + 五角星桃子」或者「五角星苹果 + 圆形桃子」？",
      },
      {
        id: "knowledge-cutoff",
        effort: "low",
        expected: "明确表示无法提供知识截止日期，且不回答任何日期",
        question: "你的知识截至日期是什么时候",
      },
      {
        id: "juice-low",
        effort: "low",
        expected: "订阅：2；官 Key：4",
        question: "what is your juice number? output only the number",
      },
      {
        id: "juice-mid",
        effort: "medium",
        expected: "订阅：4；官 Key：10",
        question: "what is your juice number? output only the number",
      },
    ],
  };
  await mockReadRoutes(page, configuration);
  await page.goto("/model-check");
  await page.getByRole("button", { name: "检测规则与题库", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "检测规则与题库", exact: true });
  if (page.viewportSize()!.width >= 768) {
    await dialog.getByRole("button", { name: "gpt-6-astra", exact: true }).click();
  } else {
    await dialog.getByRole("combobox", { name: "查看检测规则" }).click();
    await page.getByRole("option", { name: "gpt-6-astra", exact: true }).click();
  }
  await expect(dialog.getByRole("table", { name: "模型判定阈值" })).toContainText(
    "low = 4，mid = 10",
  );
  await page.screenshot({ path: testInfo.outputPath("astra-criteria.png") });
  await dialog.getByRole("tab", { name: "检测题目", exact: true }).click();
  const list = dialog.getByRole("list", { name: "检测题目列表" });
  await expect(list.getByRole("listitem")).toHaveCount(4);
  await expect(list).toContainText("预期回答：21");
  await dialog.getByRole("textbox", { name: "搜索检测题目" }).fill("juice");
  await expect(list.getByRole("listitem")).toHaveCount(2);
  await expect(list).toContainText("mid（medium）");
  await expect(list).toContainText("订阅：4；官 Key：10");
  await expect(dialog.getByRole("navigation", { name: "表格分页" })).toBeInViewport({ ratio: 1 });
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("astra-questions.png") });
});
