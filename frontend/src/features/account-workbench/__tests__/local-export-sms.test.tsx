import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { WorkbenchSMSReceipt } from "@/api";
import { WorkbenchLocalExport } from "../components/workbench-local-export";
import { workbenchKeys } from "../constants";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("本地导出的短信订单独立读取并按原作用域核对，不展示托管缓存", async () => {
  const local: WorkbenchSMSReceipt = {
    id: "local-purchase",
    task_id: "local-oauth",
    scope: "local-export",
    provider: "smsbower",
    order_id: "local-order",
    phone: "+17005550123",
    action: "acquire",
    state: "confirmed",
    updated_at: "2026-09-14T00:00:00Z",
    can_inspect: true,
  };
  const fetcher = vi.fn<typeof fetch>(async (_url, options) =>
    options?.method === "POST"
      ? Response.json({ pending: false, code_available: true, message: "原订单已收到验证码" })
      : Response.json([local]),
  );
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false }, mutations: { retry: false } },
  });
  client.setQueryDefaults(workbenchKeys.smsReceipts, { enabled: true });
  client.setQueryData(
    [...workbenchKeys.smsReceipts, "managed"],
    [{ ...local, id: "managed-purchase", scope: "managed", order_id: "managed-order" }],
  );
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchLocalExport />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("tab", { name: "短信订单" }));
  expect(await screen.findByText(/local-order/)).toBeVisible();
  expect(screen.queryByText(/managed-order/)).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledWith(
    "/api/account-workbench/sms/receipts?scope=local-export",
    expect.objectContaining({ credentials: "include" }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "核对原订单" }));
  await user.type(screen.getByLabelText("原供应商 API Key"), "isolated-provider-key");
  await user.click(screen.getByRole("button", { name: "查询原订单" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/sms/receipts/local-purchase/inspect?scope=local-export",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    ),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  view.unmount();
  client.clear();
});
