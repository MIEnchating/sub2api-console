import { expect, test } from "@playwright/test";
import type { Task, WorkbenchPreview } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("私有输入转换在桌面和手机按范围确认，成功后清空凭据并可再次转换", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const conversions: unknown[] = [];
  const previews: unknown[] = [];
  const task: Task = {
    id: "conversion-task",
    skill: "account-workbench",
    operation: "account-workbench-convert",
    status: "queued",
    progress: 0,
    message: "等待生成私有账号文件",
    result: {},
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
  };
  await page.route("**/api/account-workbench/exports", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/accounts", (route) => route.fulfill({ json: [] }));
  await page.route("**/api/tasks/conversion-task", (route) => route.fulfill({ json: task }));
  await page.route("**/api/account-workbench/exports/from-input", async (route) => {
    conversions.push(route.request().postDataJSON() as unknown);
    await route.fulfill({ json: task });
  });
  await page.route("**/api/account-workbench/preview", async (route) => {
    previews.push(route.request().postDataJSON() as unknown);
    const preview: WorkbenchPreview = {
      id: `conversion-preview-${previews.length}`,
      export_only: true,
      target: "https://sub2api.example.test",
      expires_at: new Date(Date.now() + 600000).toISOString(),
      check_after_import: false,
      model: "",
      errors: [],
      items: [
        {
          id: "0",
          index: 0,
          name: "转换账号_" + "long-name".repeat(20),
          email: "operator@example.test",
          plan_type: "plus",
          template_id: "team",
          template_name: "团队模板_" + "shared".repeat(20),
          template_revision: 3,
          group_ids: ["7"],
          duplicate: false,
        },
      ],
    };
    await route.fulfill({ json: preview });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "私有导出", exact: true }).click();
  const mode = page.getByRole("tab", { name: "输入转换为 JSON", exact: true });
  await mode.focus();
  await page.keyboard.press("Enter");
  await expect(mode).toHaveAttribute("aria-selected", "true");
  const input = page.getByRole("textbox", { name: "账号内容" });
  await input.fill("rt_private_conversion_fixture");
  await expect(page.getByRole("textbox", { name: "检测模型" })).toHaveCount(0);
  await page.getByRole("button", { name: "解析并预览" }).click();
  await expect(page.getByRole("region", { name: "账号私有转换预览" })).toBeVisible();
  expect(previews[0]).toMatchObject({
    export_only: true,
    content: "rt_private_conversion_fixture",
  });
  await expect(page.getByRole("button", { name: "确认导入 1 个账号" })).toHaveCount(0);
  await page.getByRole("button", { name: "生成私有 JSON 文件" }).click();
  const confirm = page.getByRole("dialog", { name: "确认生成私有账号文件" });
  await expect(confirm).toContainText("不创建或修改线上账号");
  expect(conversions).toHaveLength(0);
  await expect(confirm.getByRole("button", { name: "创建私有转换任务" })).toBeInViewport();
  expect(await confirm.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  expect(
    await page
      .locator('[data-slot="page-content"]')
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("input-conversion-confirm.png"),
    animations: "disabled",
  });
  await confirm.getByRole("button", { name: "创建私有转换任务" }).click();
  await expect(input).toHaveValue("");
  await expect(page.getByRole("status", { name: "等待生成私有账号文件" })).toBeVisible();
  expect(conversions).toEqual([{ preview_id: "conversion-preview-1", confirmed: true }]);
  await input.fill("rt_private_next_conversion");
  await page.getByRole("button", { name: "解析并预览" }).click();
  await expect(page.getByRole("button", { name: "生成私有 JSON 文件" })).toBeEnabled();
});
