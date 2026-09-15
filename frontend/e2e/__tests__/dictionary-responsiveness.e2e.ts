import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("排序请求未结束时立即换位，保存成功不重复读取字典", async ({ page }, testInfo) => {
  let release!: () => void;
  const saved = new Promise<void>((resolve) => {
    release = resolve;
  });
  let reads = 0;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/dictionaries/reorder") {
      expect(route.request().postDataJSON()).toEqual({
        kind: "platform",
        ids: ["gemini", "openai"],
      });
      await saved;
      await route.fulfill({ status: 204 });
      return;
    }
    if (path === "/api/dictionaries") {
      reads++;
      await route.fulfill({
        json: {
          items: ["openai", "gemini"].map((value, index) => ({
            id: value,
            kind: "platform",
            name: value,
            value,
            enabled: true,
            description: "",
            sort_order: index,
            version: 1,
            created_at: "",
            updated_at: "",
          })),
        },
      });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "字典测试" },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/config?tab=dictionaries");
  await page.getByRole("tab", { name: "字典管理", exact: true }).click();
  await page.getByRole("button", { name: "下移openai", exact: true }).click();
  const rows = page.getByRole("row");
  await expect(rows.nth(1)).toContainText("gemini");
  await expect(page.getByRole("button", { name: "下移gemini", exact: true })).toBeDisabled();
  release();
  await expect(page.getByRole("button", { name: "下移gemini", exact: true })).toBeEnabled();
  expect(reads).toBe(1);
  await page.screenshot({ path: testInfo.outputPath("dictionary-sorted.png") });
});
