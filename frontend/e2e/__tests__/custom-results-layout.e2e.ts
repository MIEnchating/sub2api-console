import { expect, test } from "@playwright/test";
import type { Task } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

for (const longEndpoint of [false, true]) {
  test(`自定义结果混合展示时卡片保持紧凑、操作可达${longEndpoint ? "，长地址自动换行" : ""}`, async ({
    page,
    colorScheme,
  }, testInfo) => {
    const endpoint = longEndpoint
      ? `https://example.invalid/${"gateway/".repeat(24)}v1`
      : "https://api.example.invalid/v1";
    const tasks: Task[] = ["precheck", "running", "animation"].map((kind) => {
      const mode = kind === "precheck" ? "precheck" : "animation";
      const target = {
        account_id: `custom-${kind}`,
        model: `gpt-6-astra-${kind}`,
        endpoint,
        platform: "openai",
      };
      return {
        id: `record-${kind}`,
        skill: "sub2api-model-animation",
        operation: `account-model-${mode}`,
        status: kind === "running" ? "running" : "succeeded",
        progress: kind === "running" ? 0 : 100,
        message: "检测",
        created_at: "2026-10-12T06:00:00Z",
        updated_at: "2026-10-12T06:00:00Z",
        result: {
          account_ids: [target.account_id],
          targets: [target],
          animations:
            kind === "running"
              ? []
              : [
                  {
                    ...target,
                    account_name: "自定义接口",
                    mode,
                    status: "succeeded",
                    request_id: `request-${kind}`,
                    duration_ms: 45300,
                    completed_at: "2026-10-12T06:00:00Z",
                    reasoning_effort: "medium",
                    ...(kind === "precheck"
                      ? { precheck: { verdict: "passed", profile_version: "v1", questions: [] } }
                      : {
                          svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 400"><rect width="640" height="400" fill="#10283f"/><circle cx="320" cy="200" r="85" fill="#38bdf8"/></svg>',
                        }),
                  },
                ],
        },
      };
    });
    await page.addInitScript(
      (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
      colorScheme,
    );
    await page.route("**/api/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      const fixtures: Record<string, unknown> = {
        ...pageFixtures,
        "/api/setup/status": { initialized: true, configuration_errors: [] },
        "/api/auth/session": { authenticated: true, username: "布局测试" },
        "/api/accounts": [],
        "/api/model-checks/animation-schedules": [],
        "/api/model-checks/animations": tasks,
        ...Object.fromEntries(tasks.map((task) => [`/api/tasks/${task.id}`, task])),
      };
      if (path.endsWith("/events"))
        await route.fulfill({ contentType: "text/event-stream", body: ": isolated fixture\n\n" });
      else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
      else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
    });
    await page.goto("/animation-check");
    await page.getByRole("tab", { name: "自定义接口", exact: true }).click();
    const records = page.getByRole("region", { name: "自定义接口检测记录" });
    const mobile = page.viewportSize()!.width < 768;
    await expect(records.getByRole("article")).toHaveCount(mobile ? 1 : 3);
    const pages = mobile ? 3 : 1;
    for (let index = 0; index < pages; index++) {
      for (const card of await records.getByRole("article").all()) {
        await expect(card.getByText(endpoint, { exact: true })).toBeVisible();
        expect(await card.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
          true,
        );
        const name = await card.getAttribute("aria-label");
        if (!longEndpoint && !name?.endsWith("-animation")) {
          expect((await card.boundingBox())!.height).toBeLessThan(280);
        }
        if (name?.endsWith("-running")) {
          const cancel = card.getByRole("button", { name: "取消任务" });
          await expect(cancel).toHaveCSS("height", "32px");
          const cardBox = (await card.boundingBox())!;
          expect((await cancel.boundingBox())!.width).toBeLessThan(cardBox.width / 2);
          await cancel.focus();
          await expect(cancel).toBeFocused();
          await expect(card.getByRole("progressbar")).toHaveCount(0);
        } else {
          const details = card.getByRole("button", { name: /查看.*检测详情/ });
          await details.scrollIntoViewIfNeeded();
          await expect(details).toBeInViewport();
        }
      }
      if (index < pages - 1) await records.getByRole("button", { name: "下一页记录" }).click();
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: testInfo.outputPath("custom-results.png") });
  });
}
