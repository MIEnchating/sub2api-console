import { expect, test } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

for (const resend of [false, true]) {
  test(`运行中的账号可${resend ? "重发邮箱验证码" : "提交验证码"}且没有浏览器画面请求`, async ({
    page,
  }) => {
    const requests: string[] = [];
    const writes: unknown[] = [];
    let submitted = false;
    await page.route("**/api/**", async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      requests.push(path);
      if (path === "/api/account-workbench/runs/run-one/login-input/item-one") {
        writes.push(request.postDataJSON());
        submitted = true;
        await route.fulfill({ json: { accepted: true } });
        return;
      }
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "协议验收" },
        "/api/dictionaries": { items: [] },
        "/api/account-workbench/templates": { revision: 1, preferred_id: "", items: [] },
        "/api/account-workbench/runs": [
          {
            id: "run-one",
            revision: 1,
            task_id: "task-one",
            status: "running",
            action: "export",
            created_at: "2026-09-20T00:00:00Z",
            updated_at: "2026-09-20T00:00:00Z",
            expires_at: "2099-01-01T00:00:00Z",
            duplicate_count: 0,
            items: [
              {
                id: "item-one",
                index: 0,
                kind: "login",
                name: "",
                email: "account@example.test",
                plan: "",
                identity_source: "login",
                template_name: "默认配置",
                status: submitted ? "authorizing" : "waiting_input",
                message: submitted ? "正在提交官方验证" : "请输入邮箱收到的六位验证码",
                login_prompt: submitted ? undefined : { id: "prompt-one", kind: "email_code" },
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
    await expect(page.getByRole("button", { name: "输入验证码" })).toBeEnabled();
    await page.getByRole("button", { name: "输入验证码" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByRole("textbox", { name: "邮箱验证码" })).toBeVisible();
    if (resend) {
      await dialog.getByRole("button", { name: "重发邮箱验证码" }).click();
    } else {
      await dialog.getByRole("textbox", { name: "邮箱验证码" }).fill("234567");
      await dialog.getByRole("button", { name: "提交验证" }).click();
    }
    await expect(dialog).toHaveCount(0);
    await expect(page.getByText("正在提交官方验证")).toBeVisible();
    expect(writes).toEqual([
      resend
        ? { prompt_id: "prompt-one", value: "", action: "resend_email" }
        : { prompt_id: "prompt-one", value: "234567" },
    ]);
    expect(requests.some((path) => path.includes("/browser/"))).toBe(false);
  });
}
