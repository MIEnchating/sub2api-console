import { expect, test } from "@playwright/test";
import { installWorkbenchFixture } from "./fixture";

test("本地RT确认转换与私有文件再生均需确认，手机及桌面可查看影响范围", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const expires = new Date(Date.now() + 600000).toISOString();
  const writes: Array<{ path: string; body: unknown }> = [];
  const task = {
    id: "local-rt-task",
    skill: "account-workbench",
    operation: "account-workbench-regenerate",
    status: "queued",
    progress: 0,
    message: "等待处理本地 RT",
    result: {},
    created_at: expires,
    updated_at: expires,
  };
  await page.route("**/api/tasks/local-rt-task", (route) => route.fulfill({ json: task }));
  await page.route("**/api/account-workbench/preview", (route) =>
    route.fulfill({
      json: {
        id: "local-rt-preview",
        scope: "local-export",
        export_only: true,
        target: "",
        expires_at: expires,
        check_after_import: false,
        model: "",
        errors: [],
        items: [
          {
            id: "0",
            index: 0,
            name: "待刷新账号",
            email: "",
            plan_type: "",
            template_id: "",
            template_name: "",
            template_revision: 0,
            group_ids: [],
            duplicate: false,
            refresh_required: true,
          },
        ],
      },
    }),
  );
  for (const path of ["exports/from-input", "exports/regenerate"])
    await page.route(`**/api/account-workbench/${path}`, (route) => {
      writes.push({ path, body: route.request().postDataJSON() as unknown });
      return route.fulfill({ json: task });
    });
  await page.route("**/api/account-workbench/local-exports", (route) =>
    route.fulfill({
      json: [
        {
          id: "local-account-artifact",
          kind: "accounts",
          count: 1,
          created_at: expires,
          expires_at: expires,
        },
      ],
    }),
  );
  await page.route("**/api/account-workbench/exports/regenerate/preview", (route) => {
    writes.push({ path: "regeneration-preview", body: route.request().postDataJSON() as unknown });
    return route.fulfill({
      json: {
        id: "local-regeneration-preview",
        scope: "local-export",
        artifact_id: "local-account-artifact",
        target: "",
        expires_at: expires,
        items: [
          {
            index: 0,
            name: "本地账号",
            email: "local@example.test",
            user_id: "user-" + "long-stable-id".repeat(20),
            workspace_id: "workspace-" + "long-stable-id".repeat(20),
            revision: "source-version",
          },
        ],
      },
    });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "本地导出", exact: true }).click();
  await page.getByRole("textbox", { name: "账号内容" }).fill("rt_isolated_refresh_only");
  await page.getByRole("button", { name: "解析并预览" }).click();
  await expect(page.getByText("确认后刷新并核对官方身份")).toBeVisible();
  await page.getByRole("button", { name: "生成私有 JSON 文件" }).click();
  const conversion = page.getByRole("dialog", { name: "确认生成私有账号文件" });
  await expect(conversion).toContainText("旧令牌可能失效");
  expect(writes).toEqual([]);
  await conversion.getByRole("button", { name: "创建私有转换任务" }).click();
  await expect.poll(() => writes.length).toBe(1);
  await page.getByRole("tab", { name: "私有文件", exact: true }).click();
  await page.getByRole("button", { name: "重新生成私有文件 local-account-artifact" }).click();
  const dialog = page.getByRole("dialog", { name: "确认重新生成授权文件" });
  await expect(dialog.getByText("范围：本地私有文件")).toBeVisible();
  expect(writes[1]).toEqual({
    path: "regeneration-preview",
    body: { scope: "local-export", artifact_id: "local-account-artifact" },
  });
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByRole("button", { name: "确认刷新并生成文件" })).toBeInViewport();
  await page.screenshot({
    path: test.info().outputPath("local-rt-regeneration.png"),
    animations: "disabled",
  });
  await dialog.getByRole("button", { name: "确认刷新并生成文件" }).click();
  await expect.poll(() => writes.length).toBe(3);
  expect(writes[2]).toEqual({
    path: "exports/regenerate",
    body: { preview_id: "local-regeneration-preview", confirmed: true },
  });
});
