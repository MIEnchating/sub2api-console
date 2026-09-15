import { expect, test } from "@playwright/test";

import { pageFixtures } from "./fixtures/page-shell";

for (const width of [1280, 980, 390]) {
  test(`批量添加底栏在 ${width}px 视口按可用空间排列，输入和预览保持对齐`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    const upstream = {
      upstream_id: "batch-layout",
      host: "batch.example.test",
      name: "批量布局测试上游",
      base_url: "https://batch.example.test",
      account_base_url: "https://batch.example.test",
      upstream_type: "sub2api",
      auth_mode: "sub2api_user_token",
      recharge_rate: "1",
      raw_balance: "10",
      balance: "10",
      groups: [],
      headers: {},
      header_names: [],
      cookie_names: [],
    };
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "批量布局回归" },
        "/api/upstreams/batch.example.test/configuration": upstream,
        "/api/groups": [{ id: "3", name: "本地 OpenAI", platform: "openai", account_count: 0 }],
        "/api/onboarding/prepare": {
          upstream,
          candidates: [
            {
              number: 1,
              host: upstream.host,
              upstream_id: upstream.upstream_id,
              upstream_name: upstream.name,
              group_id: "7",
              group_name: "待新增分组",
              platform: "openai",
              status: "active",
              multiplier: "1",
              description: null,
              bindable: true,
              can_create_key: true,
              can_bind_existing_key: false,
              bound: false,
              key_present: false,
              bound_accounts: [],
              unavailable_reason: null,
            },
          ],
        },
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto("/onboarding?host=batch.example.test&upstream_type=sub2api");
    const bar = page.getByRole("toolbar", { name: "批量添加账号" });
    const notes = bar.getByRole("textbox", { name: "批量备注（可选）" });
    const concurrency = bar.getByRole("spinbutton", { name: "并发", exact: true });
    const priority = bar.getByRole("spinbutton", { name: "优先级", exact: true });
    const preview = bar.getByRole("button", { name: "预览 0 项变更" });
    await expect(notes).toBeVisible();
    await expect(preview).toBeInViewport({ ratio: 1 });
    const noteBox = await notes.boundingBox();
    const concurrencyBox = await concurrency.boundingBox();
    const priorityBox = await priority.boundingBox();
    const actionBox = await preview.boundingBox();
    expect(noteBox && concurrencyBox && priorityBox && actionBox).toBeTruthy();
    if (!noteBox || !concurrencyBox || !priorityBox || !actionBox) return;
    if (width >= 980) {
      expect(Math.abs(noteBox.y - concurrencyBox.y)).toBeLessThanOrEqual(2);
      expect(
        Math.abs(noteBox.y + noteBox.height - actionBox.y - actionBox.height),
      ).toBeLessThanOrEqual(2);
      expect(Math.abs(priorityBox.y - actionBox.y)).toBeLessThanOrEqual(2);
      expect(noteBox.width).toBeGreaterThan(concurrencyBox.width);
      expect(concurrencyBox.width).toBeLessThanOrEqual(120);
    } else {
      expect(concurrencyBox.y).toBeGreaterThan(noteBox.y + noteBox.height);
      expect(actionBox.y).toBeGreaterThanOrEqual(concurrencyBox.y + concurrencyBox.height);
      expect(Math.abs(concurrencyBox.y - priorityBox.y)).toBeLessThanOrEqual(2);
    }
    expect(await bar.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
    await page.screenshot({ path: test.info().outputPath(`batch-bar-${width}.png`) });
  });
}
