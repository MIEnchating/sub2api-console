import { expect, test, type Route } from "@playwright/test";
import { pageFixtures } from "./fixtures/page-shell";

test("个人信息保存等待防重复，失败保留凭据，键盘重试成功后清空密码", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  let username = "operator";
  const writes: Route[] = [];
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/profile") {
      writes.push(route);
      return;
    }
    if (path.endsWith("/automation/events"))
      return route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/overview": { open_alerts: 0 },
    };
    return path in fixtures
      ? route.fulfill({ json: fixtures[path] })
      : route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
  await page.goto("/profile");
  const name = page.getByRole("textbox", { name: "账号", exact: true });
  const current = page.getByLabel("当前密码", { exact: true });
  await expect(name).toHaveValue("operator");
  await name.fill("renamed-operator");
  await current.fill("current-password");
  await page.getByLabel("新密码（可选）").fill("replacement-password");
  await page.getByLabel("确认新密码").fill("replacement-password");
  const save = page.getByRole("button", { name: "保存修改", exact: true });
  await save.click();
  await expect.poll(() => writes.length).toBe(1);
  await expect(name).toBeDisabled();
  await expect(current).toBeDisabled();
  await expect(page.getByRole("button", { name: "保存中", exact: true })).toBeDisabled();
  await page.keyboard.press("Enter");
  expect(writes).toHaveLength(1);
  await writes[0].fulfill({ status: 503, json: { detail: "私有配置暂时不可用，请重试" } });
  await expect(save).toBeEnabled();
  await expect(name).toHaveValue("renamed-operator");
  await expect(current).toHaveValue("current-password");
  // A local API may remain reachable even when the browser reports offline.
  await page.evaluate(() => window.dispatchEvent(new Event("offline")));
  await save.focus();
  await save.press("Enter");
  await expect.poll(() => writes.length).toBe(2);
  expect(writes[1].request().postDataJSON()).toEqual({
    username: "renamed-operator",
    current_password: "current-password",
    new_password: "replacement-password",
  });
  username = "renamed-operator";
  await writes[1].fulfill({ json: { authenticated: true, username } });
  await expect(save).toBeEnabled();
  await expect(current).toHaveValue("");
  await expect(page.getByLabel("新密码（可选）")).toHaveValue("");
  await expect(page.getByLabel("确认新密码")).toHaveValue("");
  await expect(name).toHaveValue(username);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: test.info().outputPath("profile-saved.png"), fullPage: true });
});
