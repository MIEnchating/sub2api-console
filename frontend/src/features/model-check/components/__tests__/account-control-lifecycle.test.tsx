import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { AccountControlState } from "@/api";
import { account } from "@/features/accounts/__tests__/fixtures";
import {
  DetectionAccountControlProvider,
  useDetectionAccountControl,
} from "../detection-account-control-provider";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function Control() {
  const control = useDetectionAccountControl();
  return (
    <button onClick={() => control?.select({ accountID: "41", action: "fuse" })}>手动熔断</button>
  );
}

it("取消慢速账号确认后中止旧读取，重新打开须读取最新状态且不提交处置", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  const detail = { ...account, name: "已有完整详情", metadata: { notes: "保留详情" } };
  client.setQueryData(["account-detail", "41"], detail);
  const state: AccountControlState = {
    id: "41",
    name: "最新账号",
    health: "healthy",
    health_status: null,
    routing_state: "healthy",
    paused: false,
    schedulable: true,
    manual_priority: null,
  };
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  const fetcher = vi
    .fn<typeof fetch>()
    .mockImplementationOnce((_input, init) => {
      signal = init?.signal;
      return new Promise<Response>((resolve) => {
        release = resolve;
      });
    })
    .mockResolvedValue(Response.json(state));
  vi.stubGlobal("fetch", fetcher);
  render(
    <QueryClientProvider client={client}>
      <DetectionAccountControlProvider>
        <Control />
      </DetectionAccountControlProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "手动熔断" }));
  await screen.findByRole("status", { name: "正在读取账号状态" });
  expect(screen.getByRole("button", { name: "确认手动熔断" })).toBeDisabled();
  try {
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () => release(Response.json({ ...account, name: "过期账号" })));
  }
  await user.click(screen.getByRole("button", { name: "手动熔断" }));
  const dialog = await screen.findByRole("dialog", { name: "手动熔断" });
  await waitFor(() => expect(dialog).toHaveTextContent("最新账号"));
  expect(dialog).not.toHaveTextContent("过期账号");
  expect(within(dialog).getByRole("button", { name: "确认手动熔断" })).toBeEnabled();
  expect(client.getQueryData(["account-detail", "41"])).toEqual(detail);
  expect(client.getQueryData(["account-detail", "41", "control-state"])).toEqual(state);
  expect(
    fetcher.mock.calls.every(
      ([input, init]) => String(input) === "/api/accounts/41/control-state" && !init?.method,
    ),
  ).toBe(true);
});
