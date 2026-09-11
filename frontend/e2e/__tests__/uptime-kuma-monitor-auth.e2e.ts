import { expect, test } from "@playwright/test";
import { setupKuma } from "../kuma-fixture";

test("选择无独立鉴权的模板后默认无鉴权，可设置并校验当前监控凭据", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "新增监控项", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新增监控项", exact: true });
  await dialog.getByLabel("监控项名称").fill("JSON 服务");
  await dialog.getByLabel("监控地址", { exact: true }).fill("https://monitor.example/health");
  await dialog.getByRole("combobox", { name: "功能模板", exact: true }).click();
  await page.getByRole("option", { name: "API JSON 模板", exact: true }).click();
  const auth = dialog.getByRole("combobox", { name: "HTTP 鉴权方式" });
  await expect(auth).toContainText("无鉴权");
  await expect(dialog.getByText(/模板鉴权/)).toHaveCount(0);
  await auth.click();
  await page.getByRole("option", { name: "Basic", exact: true }).click();
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog.getByLabel("鉴权用户名", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  await expect(dialog.getByLabel("鉴权密码 / Token", { exact: true })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(fixture.writes).toHaveLength(0);
  await dialog.getByLabel("鉴权用户名", { exact: true }).fill("health-user");
  await dialog.getByLabel("鉴权密码 / Token", { exact: true }).fill("health-password");
  await auth.click();
  await expect(page.getByRole("option", { name: "使用模板鉴权", exact: true })).toHaveCount(0);
  await page.getByRole("option", { name: "无鉴权", exact: true }).click();
  await expect(dialog.getByLabel("鉴权密码 / Token", { exact: true })).toHaveCount(0);
  await auth.click();
  await page.getByRole("option", { name: "Bearer", exact: true }).click();
  await expect(dialog.getByLabel("鉴权密码 / Token", { exact: true })).toHaveValue("");
  await dialog.getByLabel("鉴权密码 / Token", { exact: true }).fill("monitor-token");
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    monitor: {
      template_id: "a".repeat(48),
      template_revision: 2,
      template_auth_override: true,
      options: { auth_method: "bearer", auth_password: "monitor-token" },
    },
  });
});

test("监控表单按功能分区，桌面双列且窄屏单列，滚动时标题和保存按钮可见", async ({ page }) => {
  await setupKuma(page);
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "新增监控项", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新增监控项", exact: true });
  for (const name of ["基本信息", "请求设置", "鉴权设置", "检测设置"]) {
    await expect(dialog.getByRole("region", { name, exact: true })).toHaveCount(1);
  }
  const name = (await dialog.getByLabel("监控项名称").boundingBox())!;
  const type = (await dialog
    .getByRole("combobox", { name: "监控类型", exact: true })
    .boundingBox())!;
  if (page.viewportSize()!.width >= 640) {
    expect(Math.abs(name.y - type.y)).toBeLessThan(2);
    expect(type.x).toBeGreaterThan(name.x + name.width);
  } else {
    expect(type.y).toBeGreaterThan(name.y + name.height);
  }
  const body = dialog.locator('[data-slot="dialog-body"]');
  await body.hover();
  await page.mouse.wheel(0, 2000);
  await expect(dialog.getByRole("button", { name: "保存监控项" })).toBeInViewport();
  await expect(dialog.getByRole("heading", { name: "新增监控项", exact: true })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  await dialog.getByRole("combobox", { name: "功能模板", exact: true }).click();
  await page.getByRole("option", { name: "API JSON 模板", exact: true }).click();
  await dialog.getByRole("combobox", { name: "HTTP 鉴权方式" }).click();
  await page.getByRole("option", { name: "Bearer", exact: true }).click();
  await dialog.getByLabel("鉴权密码 / Token", { exact: true }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: test.info().outputPath("monitor-template-auth.png") });
});

test("编辑监控时选择模板后可明确关闭鉴权", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma");
  await page
    .getByRole("group", { name: "智谱主线 的操作" })
    .getByRole("button", { name: "编辑" })
    .click();
  const dialog = page.getByRole("dialog", { name: "编辑监控项", exact: true });
  await dialog.getByRole("combobox", { name: "功能模板", exact: true }).click();
  await page.getByRole("option", { name: "API JSON 模板", exact: true }).click();
  await dialog.getByRole("combobox", { name: "HTTP 鉴权方式" }).click();
  await page.getByRole("option", { name: "无鉴权", exact: true }).click();
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    action: "edit",
    monitor: {
      template_auth_override: true,
      options: { auth_method: "none", auth_password: "", auth_username: "" },
    },
  });
});

test("先填写监控鉴权再选择或切换模板时保留当前凭据", async ({ page }) => {
  const fixture = await setupKuma(page);
  await page.goto("/uptime-kuma");
  await page.getByRole("button", { name: "新增监控项", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新增监控项", exact: true });
  await dialog.getByLabel("监控项名称").fill("独立鉴权服务");
  await dialog.getByLabel("监控地址", { exact: true }).fill("https://monitor.example/health");
  await dialog.getByRole("combobox", { name: "HTTP 鉴权方式" }).click();
  await page.getByRole("option", { name: "Bearer", exact: true }).click();
  await dialog.getByLabel("鉴权密码 / Token", { exact: true }).fill("monitor-token");
  for (const name of ["API JSON 模板", "手动设置", "API JSON 模板"]) {
    await dialog.getByRole("combobox", { name: "功能模板", exact: true }).click();
    await page.getByRole("option", { name, exact: true }).click();
    await expect(dialog.getByRole("combobox", { name: "HTTP 鉴权方式" })).toContainText("Bearer");
    await expect(dialog.getByLabel("鉴权密码 / Token", { exact: true })).toHaveValue(
      "monitor-token",
    );
  }
  await dialog.getByRole("button", { name: "保存监控项" }).click();
  await expect(dialog).toBeHidden();
  expect(fixture.writes[0]?.value).toMatchObject({
    monitor: {
      template_auth_override: true,
      options: { auth_method: "bearer", auth_password: "monitor-token" },
    },
  });
});
