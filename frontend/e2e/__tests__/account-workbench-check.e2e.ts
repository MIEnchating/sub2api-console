import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("检测未通过时保留站点账号并提供二次确认的手动启用入口", async ({ page }) => {
  const writes: unknown[] = [];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/account-workbench/runs/batch-one/enable") {
      writes.push(route.request().postDataJSON());
      await route.fulfill({ json: {} });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "检测验收" },
      "/api/dictionaries": { items: [] },
      "/api/account-workbench/templates": { revision: 0, preferred_id: "", items: [] },
      "/api/account-workbench/runs": [
        {
          id: "batch-one",
          revision: 1,
          task_id: "task-one",
          action: "import",
          status: "needs_attention",
          created_at: "2026-09-20T00:00:00Z",
          updated_at: "2026-09-20T00:00:00Z",
          expires_at: "2099-01-01T00:00:00Z",
          duplicate_count: 0,
          items: [
            {
              id: "error-one",
              index: 0,
              kind: "codex_json",
              email: "error@example.test",
              status: "review",
              account_id: "42",
              message: "请求超时；账号已导入，未开启调度，可手动启用",
              template_name: "所选配置",
              check: { verdict: "ERROR", error: "请求超时" },
            },
            {
              id: "luna-one",
              index: 1,
              kind: "codex_json",
              email: "luna@example.test",
              status: "review",
              message: "账号已导入，检测未通过，未开启调度",
              account_id: "41",
              template_name: "所选配置",
              check: { verdict: "LUNA_LIKE", error: null },
            },
          ],
        },
      ],
    };
    if (path.endsWith("/events")) {
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      return;
    }
    if (path in fixtures) {
      await route.fulfill({ json: fixtures[path] });
      return;
    }
    await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "处理记录", exact: true }).click();
  const failure = page.getByRole("row").filter({ hasText: "error@example.test" });
  await expect(failure.getByText("检测出错", { exact: true })).toBeVisible();
  await expect(failure.getByText(/请求超时/)).toBeVisible();
  await expect(failure.getByRole("button", { name: "启用", exact: true })).toBeEnabled();
  await expect(failure.getByText(/站点账号 #42/)).toBeVisible();
  const completed = page.getByRole("row").filter({ hasText: "luna@example.test" });
  await expect(completed.getByText("更接近 Luna", { exact: true })).toBeVisible();
  await expect(completed.getByText("账号已导入，检测未通过，未开启调度")).toBeVisible();
  await expect(completed.getByRole("button", { name: "启用", exact: true })).toBeEnabled();
  await failure.getByRole("button", { name: "启用", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(/智商检测未通过/)).toBeVisible();
  expect(writes).toEqual([]);
  await dialog.getByRole("button", { name: "启用所选账号" }).click();
  await expect.poll(() => writes).toEqual([{ revision: 1, ids: ["error-one"] }]);
});
