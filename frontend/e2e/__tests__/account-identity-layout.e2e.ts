import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";

test("账号长内容与流量状态切换不改变行高，详情可通过键盘查看", async ({ page }, testInfo) => {
  let available = true;
  const name = "超长账号名称".repeat(12);
  const host = `${"long-host-".repeat(12)}example.test`;
  const groups = ["多分组内容".repeat(12), "备用分组"];
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "布局测试" },
      "/api/accounts": [
        { ...account, name, upstream_host: host, groups },
        {
          ...account,
          id: "42",
          name: "普通账号",
          upstream_host: "mdkj.lol",
          groups: ["codex-price"],
        },
      ],
      "/api/groups": [],
      "/api/policy": { advanced_policy: { manual_priority: { reserved_max: 10 } } },
      "/api/inspection/automation": { enabled: false, running: false },
      "/api/model-checks/account-statuses": [],
      "/api/accounts/traffic": {
        enabled: true,
        observed_at: new Date().toISOString(),
        accounts: [
          { account_id: "41", current_requests: 12345, waiting_requests: 0, tracked: true },
          { account_id: "42", current_requests: 0, waiting_requests: 0, tracked: true },
        ],
      },
    };
    if (path === "/api/accounts/traffic" && !available)
      await route.fulfill({ status: 503, json: { detail: "实时监控不可用" } });
    else if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path.startsWith("/api/dictionaries")) await route.fulfill({ json: { items: [] } });
    else if (path in responses) await route.fulfill({ json: responses[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/accounts");
  const row = page.locator("tbody tr").filter({ hasText: name });
  const identity = row.getByRole("cell").nth(1);
  const status = row.getByLabel("账号 41：真实请求 · 12345");
  await expect(status).toHaveText("999+ 请求");
  await expect(identity.locator("strong")).toHaveCSS("text-overflow", "ellipsis");
  const heading = identity.locator('[data-slot="account-identity-heading"]');
  await expect(heading).toHaveCSS("flex-wrap", "nowrap");
  const headingBox = await heading.boundingBox();
  const statusBox = await status.boundingBox();
  expect(statusBox!.y).toBeGreaterThanOrEqual(headingBox!.y);
  expect(statusBox!.y + statusBox!.height).toBeLessThanOrEqual(headingBox!.y + headingBox!.height);
  expect(statusBox!.x + statusBox!.width).toBeLessThanOrEqual(headingBox!.x + headingBox!.width);
  const initialHeight = (await row.boundingBox())!.height;
  const normalIdentity = page
    .locator("tbody tr")
    .filter({ hasText: "普通账号" })
    .getByRole("cell")
    .nth(1);
  const normalGroup = normalIdentity.getByText("分组：codex-price", { exact: true });
  await expect(normalGroup).toHaveCSS("border-left-width", "0px");
  expect(await normalGroup.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
    true,
  );
  const normalHost = normalIdentity.getByText("mdkj.lol", { exact: true });
  expect((await normalGroup.boundingBox())!.y).toBeGreaterThan((await normalHost.boundingBox())!.y);
  expect((await identity.locator(":scope > div").boundingBox())!.height).toBe(
    (await normalIdentity.locator(":scope > div").boundingBox())!.height,
  );
  for (const text of [name, host, `分组：${groups.join("、")}`]) {
    await identity.getByText(text, { exact: true }).focus();
    await expect(page.getByRole("tooltip")).toHaveText(text);
    await page.keyboard.press("Escape");
  }
  await status.focus();
  await expect(page.getByRole("tooltip")).toContainText("真实请求 · 12345");
  await page.keyboard.press("Escape");
  await page.screenshot({ path: testInfo.outputPath("account-layout.png") });
  available = false;
  await expect(row.getByLabel("账号 41：流量读取失败")).toBeVisible({ timeout: 12_000 });
  expect((await row.boundingBox())!.height).toBe(initialHeight);
});
