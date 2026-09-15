import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { WorkbenchSMSReceipt } from "@/api";
import { WorkbenchSMSReceipts } from "../components/workbench-sms-receipts";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
const receipt: WorkbenchSMSReceipt = {
  id: "purchase-1",
  task_id: "oauth-task-1",
  provider: "smsbower",
  order_id: "order-1",
  phone: "+17005550123",
  action: "acquire",
  state: "confirmed",
  updated_at: "2026-09-14T00:00:00Z",
  can_inspect: true,
};
function mount(fetcher: typeof fetch): QueryClient {
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchSMSReceipts />
    </QueryClientProvider>,
  );
  return client;
}

it("核对原订单仅提交原ID，等待期间清除密钥且禁止重复查询", async () => {
  let finish: (response: Response) => void = () => undefined;
  const pending = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const fetcher = vi.fn<typeof fetch>(async (_url, options) =>
    options?.method === "POST" ? pending : Response.json([receipt]),
  );
  const client = mount(fetcher);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "核对原订单" }));
  const dialog = screen.getByRole("dialog", { name: "核对原短信订单" });
  await user.type(within(dialog).getByLabelText("原供应商 API Key"), "private-provider-key");
  await user.click(within(dialog).getByRole("button", { name: "查询原订单" }));
  expect(within(dialog).getByLabelText("原供应商 API Key")).toHaveValue("");
  expect(within(dialog).getByRole("button", { name: "正在核对…" })).toBeDisabled();
  expect(fetcher).toHaveBeenCalledWith(
    "/api/account-workbench/sms/receipts/purchase-1/inspect",
    expect.objectContaining({ credentials: "include", method: "POST" }),
  );
  expect(
    JSON.stringify(
      client
        .getMutationCache()
        .getAll()
        .map((item) => item.state.variables),
    ),
  ).not.toContain("private-provider-key");
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((item) => item.state.data),
    ),
  ).not.toContain("private-provider-key");
  finish(
    Response.json({
      pending: false,
      code_available: true,
      message: "原订单已收到验证码，请在供应商查看",
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(fetcher.mock.calls.filter(([, options]) => options?.method === "POST")).toHaveLength(1);
});

it("供应商配置无效时显示字段错误且不请求订单", async () => {
  const fetcher = vi.fn<typeof fetch>(async () => Response.json([receipt]));
  mount(fetcher);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "核对原订单" }));
  await user.click(screen.getByRole("button", { name: "查询原订单" }));
  expect(await screen.findByLabelText("原供应商 API Key")).toHaveAttribute("aria-invalid", "true");
  expect(fetcher.mock.calls.some(([, options]) => options?.method === "POST")).toBe(false);
});

it("订单查询失败清除密钥并允许重新填写，已结束订单不可核对", async () => {
  mount(
    vi.fn<typeof fetch>(async (_url, options) =>
      options?.method === "POST"
        ? Response.json({ detail: "原供应商配置不匹配" }, { status: 409 })
        : Response.json([
            receipt,
            { ...receipt, id: "closed", action: "complete", can_inspect: false },
          ]),
    ),
  );
  const user = userEvent.setup();
  const buttons = await screen.findAllByRole("button", { name: "核对原订单" });
  expect(buttons[1]).toBeDisabled();
  await user.click(buttons[0]);
  await user.type(screen.getByLabelText("原供应商 API Key"), "wrong-private-key");
  await user.click(screen.getByRole("button", { name: "查询原订单" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "查询原订单" })).toBeEnabled());
  expect(screen.getByLabelText("原供应商 API Key")).toHaveValue("");
});

it("订单读取失败提供重试，空订单返回后显示空状态", async () => {
  let failed = true;
  mount(
    vi.fn<typeof fetch>(async () =>
      failed ? Response.json({ detail: "服务暂不可用" }, { status: 503 }) : Response.json([]),
    ),
  );
  const user = userEvent.setup();
  await screen.findByRole("button", { name: "重新读取" });
  failed = false;
  await user.click(screen.getByRole("button", { name: "重新读取" }));
  expect(await screen.findByText("暂无短信订单记录")).toBeInTheDocument();
});
