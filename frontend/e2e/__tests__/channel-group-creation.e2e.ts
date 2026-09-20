import { expect, test } from "@playwright/test";
import type { NewAPIChannel, NewAPIChannelGroups } from "../../src/api";
import { pageFixtures } from "./fixtures/page-shell";

test("浏览器不支持 randomUUID 时，可新建多个渠道分组并在刷新后保留", async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(crypto, "randomUUID", { value: undefined, configurable: true });
  });
  let saved: NewAPIChannelGroups = { groups: [], version: "" };
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/newapi/platforms/layout/channel-groups") {
      if (route.request().method() === "PUT") {
        saved = route.request().postDataJSON() as NewAPIChannelGroups;
        saved.version = "saved-version";
      }
      await route.fulfill({ json: saved });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "渠道分组测试" },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "test" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path.startsWith("/api/dictionaries")) await route.fulfill({ json: { items: [] } });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });

  await page.goto("/newapi/channels");
  await page.getByRole("button", { name: "渠道分组", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "渠道分组" });
  for (const name of ["生产主渠道", "备用渠道"]) {
    await dialog.getByRole("textbox", { name: "新建分组名称" }).fill(name);
    await dialog.getByRole("button", { name: "新建分组", exact: true }).click();
  }
  await expect(dialog.getByRole("textbox", { name: "分组名称 1" })).toHaveValue("生产主渠道");
  await expect(dialog.getByRole("textbox", { name: "分组名称 2" })).toHaveValue("备用渠道");
  await dialog.getByRole("button", { name: "保存分组" }).click();
  await expect(dialog).not.toBeVisible();
  expect(saved.groups).toHaveLength(2);
  expect(saved.groups[0].id).not.toBe(saved.groups[1].id);
  for (const group of saved.groups) expect(group.id).toMatch(/^[a-zA-Z0-9_-]{1,80}$/);

  await page.reload();
  await page.getByRole("button", { name: "渠道分组", exact: true }).click();
  await expect(dialog.getByRole("textbox", { name: "分组名称 1" })).toHaveValue("生产主渠道");
  await expect(dialog.getByRole("textbox", { name: "分组名称 2" })).toHaveValue("备用渠道");
});

test("分组卡片与渠道选择适配窄屏，支持键盘添加且保存按钮始终可见", async ({
  page,
  colorScheme,
}) => {
  const longName = "生产渠道-".repeat(30);
  const channels: NewAPIChannel[] = Array.from({ length: 30 }, (_, index) => ({
    id: String(index + 1),
    name: index === 0 ? longName : `业务渠道 ${index + 1}`,
    type: 59,
    status: 1,
    groups: ["default"],
    models: ["gpt-5"],
    version: "v1",
  }));
  let saved: NewAPIChannelGroups = {
    groups: Array.from({ length: 8 }, (_, index) => ({
      id: `group-${index}`,
      name: `渠道分组 ${index + 1}`,
      channel_ids: ["2"],
    })),
    version: "v1",
  };
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/channel-groups")) {
      if (route.request().method() === "PUT")
        saved = route.request().postDataJSON() as NewAPIChannelGroups;
      await route.fulfill({ json: saved });
      return;
    }
    const fixtures: Record<string, unknown> = {
      ...pageFixtures,
      "/api/setup/status": { initialized: true, configuration_errors: [] },
      "/api/auth/session": { authenticated: true, username: "分组交互测试" },
      "/api/preferences/navigation": { hidden_item_ids: [], version: "test" },
      "/api/inspection/automation": {
        enabled: false,
        running: false,
        traffic_collection: { enabled: false },
      },
      "/api/newapi/platforms/layout/channels": { items: channels, total: channels.length },
    };
    if (path.endsWith("/events"))
      await route.fulfill({ contentType: "text/event-stream", body: ": fixture\n\n" });
    else if (path.startsWith("/api/dictionaries")) await route.fulfill({ json: { items: [] } });
    else if (path in fixtures) await route.fulfill({ json: fixtures[path] });
    else await route.fulfill({ status: 503, json: { detail: "隔离测试未配置接口" } });
  });
  await page.goto("/newapi/channels");
  await page.getByRole("button", { name: "渠道分组", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "渠道分组", exact: true });
  const save = dialog.getByRole("button", { name: "保存分组" });
  await expect(save).toBeInViewport();
  await expect(dialog.locator('[data-slot="dialog-body"]')).toHaveCSS("overflow-y", "auto");
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  const first = dialog.getByRole("group", { name: "分组 1", exact: true });
  const add = first.getByRole("button", { name: "添加渠道", exact: true });
  await add.click();
  const picker = page.getByRole("dialog", { name: "添加渠道到分组" });
  const region = picker.getByRole("region", { name: "可添加的渠道" });
  await expect(region).toHaveCSS("overflow-y", "auto");
  expect(await picker.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  const member = picker.getByRole("checkbox", { name: "选择渠道 业务渠道 2（2）" });
  await expect(member).toBeChecked();
  await expect(member).toBeDisabled();
  const search = picker.getByRole("textbox", { name: "搜索本页渠道名称或 ID" });
  await search.fill("没有该渠道");
  await expect(picker.getByText("本页没有匹配的渠道")).toBeVisible();
  await search.clear();
  const choice = picker.getByRole("checkbox", { name: `选择渠道 ${longName}（1）` });
  await choice.focus();
  await page.keyboard.press("Space");
  await expect(choice).toBeChecked();
  const confirm = picker.getByRole("button", { name: "添加到分组（1）" });
  await expect(confirm).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("channel-picker.png") });
  await confirm.click();
  await expect(picker).not.toBeVisible();
  await expect(add).toBeFocused();
  await first.locator("summary").click();
  await expect(first.getByText(longName, { exact: true })).toBeVisible();
  await first.getByText(longName, { exact: true }).focus();
  await page.keyboard.press("Tab");
  await page.keyboard.press("Shift+Tab");
  await expect(page.getByRole("tooltip")).toContainText(longName);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("tooltip")).not.toBeVisible();
  await expect(dialog).toBeVisible();
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  await expect(save).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("channel-groups.png") });
  await save.click();
  await expect(dialog).not.toBeVisible();
  expect(saved.groups[0].channel_ids).toEqual(["2", "1"]);
});
