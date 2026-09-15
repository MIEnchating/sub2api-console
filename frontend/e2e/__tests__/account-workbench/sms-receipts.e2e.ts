import { expect, test } from "@playwright/test";
import type { WorkbenchSMSReceipt } from "../../../src/api";
import { installWorkbenchFixture } from "./fixture";

test("短信原订单核对在桌面与移动端清除提交密钥，结束订单禁用且结果不显示验证码", async ({
  page,
  colorScheme,
}) => {
  await page.addInitScript(
    (theme) => localStorage.setItem("sub2api-console-theme", theme ?? "light"),
    colorScheme,
  );
  await installWorkbenchFixture(page);
  const orderID = "sms-order-" + "original-order-".repeat(20);
  const receipts: WorkbenchSMSReceipt[] = [
    {
      id: "receipt-original",
      task_id: "oauth-original-task",
      provider: "smsbower",
      order_id: orderID,
      phone: "+447700900123",
      action: "ready",
      state: "confirmed",
      updated_at: "2026-09-14T00:00:00Z",
      can_inspect: true,
    },
    {
      id: "receipt-ended",
      task_id: "oauth-ended-task",
      provider: "luban",
      order_id: "ended-order",
      phone: "+447700900456",
      action: "release",
      state: "confirmed",
      updated_at: "2026-09-14T00:00:00Z",
      can_inspect: false,
    },
  ];
  await page.route("**/api/account-workbench/sms/receipts", (route) =>
    route.fulfill({ json: receipts }),
  );
  let release: (() => void) | undefined;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  const requests: unknown[] = [];
  await page.route(
    "**/api/account-workbench/sms/receipts/receipt-original/inspect",
    async (route) => {
      requests.push(route.request().postDataJSON() as unknown);
      await pending;
      await route.fulfill({
        json: {
          pending: false,
          code_available: true,
          message: "原订单已有短信，请在供应商查看后人工输入原登录页面",
        },
      });
    },
  );
  await page.goto("/account-workbench");
  await page.getByRole("tab", { name: "授权登录", exact: true }).click();
  await page.getByRole("tab", { name: "短信订单", exact: true }).click();
  const records = page.getByRole("region", { name: "短信订单记录" });
  await expect(records.getByRole("listitem")).toHaveCount(2);
  const ended = records.getByRole("listitem").filter({ hasText: "ended-order" });
  await expect(ended.getByRole("button", { name: "核对原订单" })).toBeDisabled();
  const original = records.getByRole("listitem").filter({ hasText: orderID });
  const inspectButton = original.getByRole("button", { name: "核对原订单" });
  await inspectButton.focus();
  await page.keyboard.press("Enter");
  const dialog = page.getByRole("dialog", { name: "核对原短信订单" });
  await expect(dialog).toContainText(orderID);
  await expect(dialog.getByRole("button", { name: "查询原订单" })).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "返回" })).toBeInViewport();
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  const key = dialog.getByLabel("原供应商 API Key");
  await expect(key).toHaveAttribute("type", "password");
  await key.fill("sms-isolated-api-key-20260914");
  const request = page.waitForRequest(
    "**/api/account-workbench/sms/receipts/receipt-original/inspect",
  );
  await dialog.getByRole("button", { name: "查询原订单" }).click();
  await request;
  await expect(key).toHaveValue("");
  await expect(key).toBeDisabled();
  await expect(dialog.getByRole("button", { name: "正在核对…" })).toBeDisabled();
  expect(requests).toEqual([
    {
      provider: "smsbower",
      api_key: "sms-isolated-api-key-20260914",
      service_id: "",
      custom_entries: "",
    },
  ]);
  await page.screenshot({
    path: test.info().outputPath("sms-original-order-pending.png"),
    animations: "disabled",
  });
  release?.();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("原订单已有短信，请在供应商查看后人工输入原登录页面")).toBeVisible();
  await expect(records.getByRole("textbox")).toHaveCount(0);
  await expect(records.getByRole("link")).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText("sms-isolated-api-key-20260914");
  expect(
    await page.evaluate(() => JSON.stringify({ ...localStorage, ...sessionStorage })),
  ).not.toContain("sms-isolated-api-key-20260914");
  await inspectButton.click();
  await expect(dialog.getByLabel("原供应商 API Key")).toHaveValue("");
  await dialog.getByRole("button", { name: "返回" }).click();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({
    path: test.info().outputPath("sms-original-order-result.png"),
    animations: "disabled",
  });
});
