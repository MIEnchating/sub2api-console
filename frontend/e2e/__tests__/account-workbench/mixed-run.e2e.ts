import { expect, test } from "@playwright/test";
import type {
  Task,
  WorkbenchPreview,
  WorkbenchRunInput,
  WorkbenchRunPreview,
  WorkbenchRunRow,
  WorkbenchRunView,
} from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

for (const exportOnly of [false, true]) {
  test(`混合运行${exportOnly ? "私有转换" : "线上导入"}保持原序号并完成授权后统一确认`, async ({
    page,
    colorScheme,
  }) => {
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
      colorScheme,
    );
    await installWorkbenchFixture(page);
    const inputs: WorkbenchRunInput[] = [];
    const starts: unknown[] = [];
    const writes: { kind: string; body: unknown }[] = [];
    let ready = false;
    const rows: WorkbenchRunRow[] = [
      {
        index: 0,
        kind: "codex_json",
        name: "JSON 账号_" + "long-workspace".repeat(18),
        email: "json@example.test",
        has_password: false,
        has_totp: false,
        has_proxy: false,
        status: "queued",
        message: "等待处理",
      },
      {
        index: 1,
        kind: "refresh_token",
        name: "RT 账号",
        has_password: false,
        has_totp: false,
        has_proxy: false,
        status: "queued",
        message: "等待处理",
      },
      {
        index: 2,
        kind: "oauth_login",
        name: "登录账号",
        email: "operator@example.test",
        has_password: true,
        has_totp: false,
        has_proxy: true,
        status: "queued",
        message: "等待授权",
      },
    ];
    const task: Task = {
      id: "mixed-final-task",
      skill: "account-workbench",
      operation: exportOnly ? "account-workbench-convert" : "account-workbench-import",
      status: "queued",
      progress: 0,
      message: "等待执行混合运行结果",
      result: {},
      created_at: "2026-09-14T00:00:00Z",
      updated_at: "2026-09-14T00:00:00Z",
    };
    const expires = (): string => new Date(Date.now() + 600000).toISOString();
    await page.route("**/api/account-workbench/runs/preview", async (route) => {
      inputs.push(route.request().postDataJSON() as WorkbenchRunInput);
      const preview: WorkbenchRunPreview = {
        id: "mixed-preview",
        target: "https://sub2api.example.test",
        expires_at: expires(),
        export_only: exportOnly,
        items: rows,
        errors: [],
      };
      await route.fulfill({ json: preview });
    });
    await page.route("**/api/account-workbench/runs", async (route) => {
      starts.push(route.request().postDataJSON() as unknown);
      const run: WorkbenchRunView = {
        id: "mixed-run",
        task_id: "mixed-task",
        status: "waiting_input",
        message: "等待登录账号完成授权",
        expires_at: expires(),
        export_only: exportOnly,
        current_oauth_id: "oauth-fixture",
        available: 2,
        items: rows,
        errors: [],
      };
      await route.fulfill({ json: run });
    });
    await page.route("**/api/account-workbench/runs/mixed-run", async (route) => {
      const run: WorkbenchRunView = {
        id: "mixed-run",
        task_id: "mixed-task",
        status: ready ? "ready" : "waiting_input",
        message: ready ? "全部账号准备完成" : "等待登录账号完成授权",
        expires_at: expires(),
        export_only: exportOnly,
        current_oauth_id: ready ? undefined : "oauth-fixture",
        available: ready ? 3 : 2,
        items: rows,
        errors: [],
      };
      await route.fulfill({
        json: route.request().method() === "DELETE" ? { cancelled: true } : run,
      });
    });
    await page.route("**/api/account-workbench/oauth/oauth-fixture/finish", async (route) => {
      ready = true;
      await route.fallback();
    });
    await page.route("**/api/account-workbench/runs/mixed-run/preview", async (route) => {
      const preview: WorkbenchPreview = {
        id: "mixed-result",
        target: "https://sub2api.example.test",
        expires_at: expires(),
        export_only: exportOnly,
        check_after_import: false,
        model: "",
        errors: [],
        items: [
          {
            id: "2",
            index: 2,
            name: "登录账号",
            email: "operator@example.test",
            plan_type: "plus",
            template_id: "",
            template_name: "自动匹配",
            template_revision: 0,
            group_ids: [],
            duplicate: false,
          },
        ],
      };
      await route.fulfill({ json: preview });
    });
    await page.route("**/api/account-workbench/import", async (route) => {
      writes.push({ kind: "import", body: route.request().postDataJSON() as unknown });
      await route.fulfill({ json: task });
    });
    await page.route("**/api/account-workbench/exports/from-input", async (route) => {
      writes.push({ kind: "convert", body: route.request().postDataJSON() as unknown });
      await route.fulfill({ json: task });
    });
    await page.route("**/api/tasks/mixed-final-task", (route) => route.fulfill({ json: task }));
    await page.goto("/account-workbench");
    const mode = page.getByRole("tab", { name: "混合运行", exact: true });
    await mode.focus();
    await page.keyboard.press("Enter");
    await expect(mode).toHaveAttribute("aria-selected", "true");
    if (exportOnly) {
      await page.getByRole("combobox", { name: "处理方式" }).click();
      await page.getByRole("option", { name: "生成私有 JSON", exact: true }).click();
    }
    const content =
      '{"access_token":"fixture-access"}\nrt_fixture_refresh\noperator@example.test----fixture-password';
    await page.getByRole("textbox", { name: "混合账号内容" }).fill(content);
    await page
      .getByLabel("本批登录代理", { exact: true })
      .fill("https://user:fixture-secret@proxy.example.test:443");
    await page.getByRole("button", { name: "解析混合运行范围" }).click();
    const preview = page.getByRole("region", { name: "混合运行预览" });
    await expect(preview).toBeVisible();
    await expect(page.getByRole("textbox", { name: "混合账号内容" })).toHaveCount(0);
    expect(inputs[0]).toMatchObject({
      content,
      export_only: exportOnly,
      proxy_url: "https://user:fixture-secret@proxy.example.test:443",
    });
    expect(starts).toHaveLength(0);
    expect(await preview.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await preview.getByRole("button", { name: "确认处理 3 项" }).click();
    const confirm = page.getByRole("dialog", { name: "确认开始混合运行" });
    await expect(confirm).toContainText("3 项账号资料");
    await expect(confirm.getByRole("button", { name: "开始混合运行" })).toBeInViewport();
    await page.screenshot({
      path: test.info().outputPath("mixed-run-confirm.png"),
      animations: "disabled",
    });
    await confirm.getByRole("button", { name: "开始混合运行" }).click();
    expect(starts).toEqual([{ preview_id: "mixed-preview", confirmed: true }]);
    await expect(page.getByRole("button", { name: "上游登录页面" })).toBeVisible();
    await page.getByRole("button", { name: "登录完成，验证授权" }).click();
    await page
      .getByRole("button", {
        name: exportOnly ? "预览私有转换结果" : "预览可导入账号",
        exact: true,
      })
      .click();
    if (exportOnly) {
      await expect(page.getByRole("button", { name: "确认导入 1 个账号" })).toHaveCount(0);
      await page.getByRole("button", { name: "生成私有 JSON 文件" }).click();
      expect(writes).toHaveLength(0);
      await page.getByRole("button", { name: "创建私有转换任务" }).click();
    } else {
      await page.getByRole("button", { name: "确认导入 1 个账号" }).click();
      expect(writes).toHaveLength(0);
      await page.getByRole("button", { name: "创建导入任务" }).click();
    }
    await expect(page.getByRole("status", { name: "等待执行混合运行结果" })).toBeVisible();
    expect(writes).toEqual([
      {
        kind: exportOnly ? "convert" : "import",
        body: { preview_id: "mixed-result", confirmed: true },
      },
    ]);
    await expect(page.getByRole("textbox", { name: "混合账号内容" })).toHaveValue("");
  });
}
