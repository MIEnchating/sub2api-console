import { expect, test } from "@playwright/test";
import { pageFixtures } from "../fixtures/page-shell";

test("从恢复鉴权打开浏览器后可键盘操作并关闭返回原弹窗", async ({ page }) => {
  const inputs: Record<string, unknown>[] = [];
  let cancelled = false;
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const method = route.request().method();
    const browser = {
      id: "browser-1",
      task_id: "task-1",
      host: "login.example.test",
      status: "waiting",
      message: "等待登录",
      expires_at: "2030-01-01T00:00:00Z",
      width: 1100,
      height: 760,
      image:
        "data:image/svg+xml," +
        encodeURIComponent(
          '<svg xmlns="http://www.w3.org/2000/svg" width="1100" height="760"><rect width="1100" height="760" fill="white"/><text x="40" y="60">Isolated login page</text></svg>',
        ),
    };
    if (path.endsWith("/browser/browser-1/input")) {
      inputs.push(route.request().postDataJSON() as Record<string, unknown>);
      await route.fulfill({ json: { accepted: true } });
      return;
    }
    if (path === "/api/auth-recovery/browser" || path === "/api/auth-recovery/browser/browser-1") {
      if (method === "DELETE") cancelled = true;
      await route.fulfill({ json: method === "DELETE" ? { cancelled: true } : browser });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "浏览器验证测试" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/upstreams": {
        hosts: [
          {
            upstream_id: "up_test",
            host: "login.example.test",
            hosts: ["login.example.test"],
            name: "测试上游",
            base_url: "https://login.example.test",
            upstream_type: "sub2api",
            auth_status: "鉴权失效",
            account_count: 0,
            group_count: 0,
            raw_balance: null,
            balance: null,
            display_balance: null,
            balance_unit: null,
            recharge_rate: "1",
            balance_status: "未读取",
            checked_at: null,
          },
        ],
        total_hosts: 1,
        authenticated_hosts: 0,
        recovery_required: 1,
        source: "test",
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/upstreams");
  await page.getByRole("button", { name: "更多操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "恢复鉴权", exact: true }).click();
  await page.getByRole("button", { name: "打开浏览器手动验证" }).click();
  const dialog = page.getByRole("dialog", { name: "浏览器手动验证 · login.example.test" });
  const surface = dialog.getByRole("button", { name: "上游登录页面" });
  await expect(surface).toBeVisible();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await surface.focus();
  await page.keyboard.type("hello");
  await expect
    .poll(() =>
      inputs
        .filter((input) => input.kind === "text")
        .map((input) => input.text)
        .join(""),
    )
    .toBe("hello");
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: "上一个输入框" })).toBeFocused();
  await dialog.getByRole("button", { name: "关闭验证" }).click();
  await expect(dialog).toBeHidden();
  await expect.poll(() => cancelled).toBe(true);
  await expect(page.getByRole("dialog", { name: "恢复鉴权", exact: true })).toBeVisible();
});
