import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import { account } from "../../src/features/accounts/__tests__/fixtures";
import { pageFixtures } from "./fixtures/page-shell";

// Keep historical fixtures on the displayed Beijing calendar day.
test.beforeEach(async ({ page }) => {
  await page.clock.setFixedTime(new Date("2026-10-09T08:00:00Z"));
});

// The same complete upstream reply is exercised through the Go artifact parser.
const source = readFileSync(
  new URL(
    "../../../backend/internal/modelcheck/__tests__/testdata/pelican-markdown.txt",
    import.meta.url,
  ),
  "utf8",
);
const html = source.slice(source.indexOf("<!DOCTYPE html>"), source.indexOf("</html>") + 7);

test("带说明文字的鹈鹕动画保留原文，隔离预览支持脚本、暂停和速度控件", async ({ page }) => {
  const task = {
    id: "markdown-pelican",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "succeeded",
    progress: 100,
    message: "动画完成",
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:00Z",
    result: {
      animations: [
        {
          account_id: "41",
          account_name: "鹈鹕样本",
          model: "test-model",
          status: "succeeded",
          request_id: "pelican-41",
          duration_ms: 1000,
          completed_at: "2026-10-09T00:00:00Z",
          html,
          source,
        },
      ],
    },
  };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "样本测试" },
      "/api/accounts": [{ ...account, id: "41", name: "鹈鹕样本", platform: "openai" }],
      "/api/model-checks/animation-schedules": [],
      "/api/model-checks/animations": [task],
      "/api/tasks/markdown-pelican": task,
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置此接口" } });
  });
  await page.goto("/animation-check");
  await page.getByRole("button", { name: "放大查看 鹈鹕样本 的动画" }).click();
  const dialog = page.getByRole("dialog", { name: "动画预览", exact: true });
  const frame = dialog.locator("iframe");
  await expect(frame).toHaveAttribute("sandbox", "allow-scripts");
  const scene = frame.contentFrame();
  await expect(scene.locator("#rearSpokes line")).toHaveCount(16);
  const initial = await scene.locator("#pelicanBody").getAttribute("cy");
  await expect.poll(() => scene.locator("#pelicanBody").getAttribute("cy")).not.toBe(initial);
  await scene.getByRole("button", { name: "暂停", exact: true }).click();
  await expect(scene.getByRole("button", { name: "继续", exact: true })).toBeVisible();
  await scene.getByRole("slider", { name: "速度：" }).fill("1.8");
  await expect(scene.getByRole("slider", { name: "速度：" })).toHaveValue("1.8");
  await scene.getByRole("button", { name: "继续", exact: true }).click();
  await expect(scene.getByRole("button", { name: "暂停", exact: true })).toBeVisible();
  await dialog.getByRole("tab", { name: "代码", exact: true }).click();
  await expect(dialog.getByRole("tabpanel", { name: "代码", exact: true })).toContainText(
    "将下面的代码保存为 .html 文件",
  );
  await expect(dialog.getByRole("tabpanel", { name: "代码", exact: true })).toContainText(
    "## 动画效果",
  );
});
