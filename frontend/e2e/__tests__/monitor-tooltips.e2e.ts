import { expect, test } from "@playwright/test";

import { monitor } from "../../src/features/uptime-kuma/components/__tests__/fixtures";
import { setupKuma } from "../kuma-fixture";

test("长监控名称显示共享提示，筛选禁用的分组按钮仍能解释原因", async ({ page }) => {
  const name = "用于检查悬浮提示的长监控名称".repeat(6);
  const group = { ...monitor, id: 31, key: "id:31", name: "openai", type: "group" };
  const child = { ...monitor, id: 32, key: "id:32", name, parent: 31 };
  await setupKuma(page, [group, child]);
  await page.goto("/uptime-kuma");

  const detail = page.getByRole("button", { name: `查看 ${name} 详情`, exact: true });
  await expect(detail).toHaveCSS("height", "32px");
  await detail.hover();
  await expect(page.locator('[data-slot="tooltip-content"]')).toHaveText(name);
  await expect(page.locator('[data-slot="tooltip-content"]')).toBeInViewport({ ratio: 1 });
  await page.keyboard.press("Escape");
  await detail.focus();
  await detail.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");

  const search = page.getByRole("textbox", { name: "搜索监控项或地址" });
  await search.fill("用于检查悬浮提示");
  const toggle = page.getByRole("button", { name: "收起分组 openai", exact: true });
  await expect(toggle).toBeDisabled();
  await expect(toggle).toHaveAttribute("aria-description", "筛选时自动展开匹配分组");
  await page.locator('[data-slot="tooltip-trigger"]').filter({ has: toggle }).hover();
  await expect(page.locator('[data-slot="tooltip-content"]')).toHaveText("筛选时自动展开匹配分组");
  await expect(toggle).toHaveCSS("height", "32px");
  await expect(page.locator("[title]")).toHaveCount(0);
});
