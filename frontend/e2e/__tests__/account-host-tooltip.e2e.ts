import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";

test("从账号地址向上移入浮层时保留地址并可选中复制", async ({ page }) => {
  const host = "app.haoshuai.cc.cd";
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const responses: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "浮层测试" },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "test" },
      "/api/accounts": [{ ...account, name: "地址复制测试账号", upstream_host: host }],
      "/api/groups": [],
      "/api/policy": { advanced_policy: { manual_priority: { reserved_max: 10 } } },
      "/api/inspection/automation": { enabled: false, running: false },
      "/api/model-checks/account-statuses": [],
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path.startsWith("/api/dictionaries")) await route.fulfill({ json: { items: [] } });
    else if (path in responses) await route.fulfill({ json: responses[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/accounts");
  const row = page.locator("tbody tr").filter({ hasText: "地址复制测试账号" });
  const address = row.getByText(host, { exact: true });
  await address.hover();
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toHaveText(host);
  const triggerBox = (await address.boundingBox())!;
  const popupBox = (await tooltip.boundingBox())!;
  const x = triggerBox.x + triggerBox.width / 2;
  const gapY = (triggerBox.y + popupBox.y + popupBox.height) / 2;
  await page.mouse.move(x, gapY, { steps: 8 });
  // 等到浏览器处理此次指针移动，确认间隙不会关闭地址浮层。
  await expect(tooltip).toBeVisible();
  expect(
    await tooltip.evaluate(
      (element, point) => element.contains(document.elementFromPoint(point.x, point.y)),
      { x, y: gapY },
    ),
  ).toBe(true);
  await page.mouse.move(x, popupBox.y + popupBox.height / 2, { steps: 8 });
  await expect(tooltip).toHaveText(host);
  await tooltip.click({ clickCount: 3 });
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe(host);
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.keyboard.press("Control+c");
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(host);
  await page.keyboard.press("Escape");
  await page.mouse.move(0, 0);
  await expect(tooltip).toHaveCount(0);
});
