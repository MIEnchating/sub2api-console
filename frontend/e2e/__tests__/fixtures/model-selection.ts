import { expect, type Page } from "@playwright/test";

export async function selectDetectionModels(page: Page, models: string[]): Promise<void> {
  const trigger = page.getByRole("combobox", { name: "检测模型", exact: true });
  if ((await trigger.getAttribute("aria-expanded")) !== "true") await trigger.click();
  const clear = page.getByRole("button", { name: "清空模型", exact: true });
  if (await clear.isEnabled()) await clear.click();
  for (const model of models) {
    await page.getByRole("combobox", { name: "搜索或输入模型 ID", exact: true }).fill(model);
    const existing = page.getByRole("option", { name: model, exact: true });
    const option = existing.or(
      page.getByRole("option", { name: `使用模型 ${model}`, exact: true }),
    );
    await expect(option).toBeVisible();
    await option.click();
  }
  await page.keyboard.press("Escape");
}
