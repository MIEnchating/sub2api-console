import { expect, test, type Route } from "@playwright/test";
import type { PolicyUpdatePayload } from "../../src/api";
import { policy } from "./fixtures/settings";

test.beforeEach(async ({ page, colorScheme }) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const fixtures: Record<string, unknown> = {
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "策略编辑测试" },
      "/api/config": { probes_enabled: true },
      "/api/policy": policy,
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/dictionaries": { items: [] },
    };
    if (path.endsWith("/automation/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
});

test("保存期间继续输入保留新草稿，键盘再次保存使用已确认版本", async ({ page }) => {
  const pending: Route[] = [];
  const writes: PolicyUpdatePayload[] = [];
  await page.route("**/api/policy", async (route) => {
    if (route.request().method() !== "PUT") return route.fulfill({ json: policy });
    writes.push(route.request().postDataJSON() as PolicyUpdatePayload);
    pending.push(route);
  });
  await page.goto("/policy");
  const field = page.getByRole("spinbutton", { name: "每组总权重预算" });
  const save = page.getByRole("button", { name: "保存策略", exact: true });
  await field.fill("500");
  await save.click();
  await expect.poll(() => pending.length).toBe(1);
  await expect(page.getByRole("button", { name: "刷新策略" })).toBeDisabled();
  await field.fill("600");
  await pending[0].fulfill({
    json: { ...policy, revision: "saved-revision", advanced_policy: writes[0].advanced_policy },
  });
  await expect(save).toBeEnabled();
  await expect(field).toHaveValue("600");
  await expect(page.getByText("策略已保存，后续修改尚未保存", { exact: true })).toBeVisible();
  await save.focus();
  await save.press("Enter");
  await expect.poll(() => writes.length).toBe(2);
  expect(writes[1]).toMatchObject({
    expected_revision: "saved-revision",
    advanced_policy: { weights: { budget: 600 } },
  });
  await pending[1].fulfill({
    json: { ...policy, revision: "second-revision", advanced_policy: writes[1].advanced_policy },
  });
  await expect(save).toBeEnabled();
  await expect(field).toHaveValue("600");
  await page.screenshot({ path: test.info().outputPath("policy-draft-saved.png") });
});

test("刷新期间新输入保留原版本，外部修改后保存提示冲突且不丢草稿", async ({ page }) => {
  let hold = false;
  const pending: Route[] = [];
  const writes: PolicyUpdatePayload[] = [];
  await page.route("**/api/policy", async (route) => {
    if (route.request().method() === "PUT") {
      writes.push(route.request().postDataJSON() as PolicyUpdatePayload);
      await route.fulfill({
        status: 409,
        json: { detail: "策略已被其他操作修改，请刷新策略后重试" },
      });
    } else if (hold) pending.push(route);
    else await route.fulfill({ json: policy });
  });
  await page.goto("/policy");
  const field = page.getByRole("spinbutton", { name: "每组总权重预算" });
  await expect(field).toHaveValue("1000");
  hold = true;
  await page.getByRole("button", { name: "刷新策略" }).click();
  await expect.poll(() => pending.length).toBe(1);
  await field.fill("650");
  await pending[0].fulfill({ json: { ...policy, revision: "external-revision" } });
  await expect(page.getByRole("button", { name: "刷新策略" })).toBeEnabled();
  await expect(field).toHaveValue("650");
  await page.getByRole("button", { name: "保存策略", exact: true }).click();
  await expect(
    page.getByText("策略已被其他操作修改，请刷新策略后重试", { exact: true }),
  ).toBeVisible();
  expect(writes[0].expected_revision).toBe(policy.revision);
  await expect(field).toHaveValue("650");
});
