import { afterEach, expect, test, vi } from "vitest";
import { api } from "../api";

afterEach(() => vi.unstubAllGlobals());

test("独立短信订单核对将范围放在查询参数，凭据仍仅放请求体", async () => {
  const fetch = vi
    .fn<typeof globalThis.fetch>()
    .mockResolvedValue(
      Response.json({ pending: true, code_available: false, message: "等待验证码" }),
    );
  vi.stubGlobal("fetch", fetch);

  await api.inspectWorkbenchSMSReceipt("receipt/1", {
    provider: "smsbower",
    api_key: "isolated-supplier-key",
    scope: "local-export",
  });

  expect(fetch).toHaveBeenCalledWith(
    "/api/account-workbench/sms/receipts/receipt%2F1/inspect?scope=local-export",
    expect.objectContaining({
      method: "POST",
      credentials: "include",
      cache: "no-store",
      body: JSON.stringify({ provider: "smsbower", api_key: "isolated-supplier-key" }),
    }),
  );
});

test("未指定短信订单范围时保留默认管理范围请求", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(Response.json([]));
  vi.stubGlobal("fetch", fetch);

  await api.workbenchSMSReceipts();

  expect(fetch).toHaveBeenCalledWith(
    "/api/account-workbench/sms/receipts",
    expect.objectContaining({ credentials: "include", cache: "no-store" }),
  );
});
