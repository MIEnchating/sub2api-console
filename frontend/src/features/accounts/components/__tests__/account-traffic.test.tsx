import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api, type AccountTrafficSnapshot } from "@/api";
import {
  AccountTrafficBadge,
  AccountTrafficProvider,
  SelectTrafficAccounts,
} from "../account-traffic";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});
function setup(
  snapshot?: AccountTrafficSnapshot,
  accountIDs = ["41", "42"],
): {
  client: QueryClient;
  select: ReturnType<typeof vi.fn>;
} {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  if (snapshot) client.setQueryData(["accounts", "live-traffic"], snapshot);
  const select = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <AccountTrafficProvider>
        <AccountTrafficBadge accountID="41" />
        <AccountTrafficBadge accountID="42" />
        <SelectTrafficAccounts accountIDs={accountIDs} onSelect={select} />
      </AccountTrafficProvider>
    </QueryClientProvider>,
  );
  return { client, select };
}
function snapshot(): AccountTrafficSnapshot {
  return {
    enabled: true,
    observed_at: new Date().toISOString(),
    accounts: [
      { account_id: "41", current_requests: 2, waiting_requests: 0, tracked: true },
      { account_id: "42", current_requests: 0, waiting_requests: 3, tracked: true },
    ],
  };
}

it("账号正在处理请求时展示实时数，排队账号不参与选择", async () => {
  const user = userEvent.setup();
  const view = setup(snapshot());
  expect(screen.getByLabelText("账号 41：真实请求 · 2")).toBeVisible();
  expect(screen.getByLabelText("账号 42：未观测到请求")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "选择实时流量（1）" }));
  expect(view.select).toHaveBeenCalledWith(["41"]);
});
it("实时并发归零后取消可选资格，不沿用旧流量状态", async () => {
  const view = setup(snapshot());
  await act(async () =>
    view.client.setQueryData(["accounts", "live-traffic"], {
      ...snapshot(),
      accounts: [{ account_id: "41", current_requests: 0, waiting_requests: 0, tracked: true }],
    }),
  );
  expect(await screen.findByRole("button", { name: "选择实时流量（0）" })).toBeDisabled();
  expect(screen.getByLabelText("账号 41：未观测到请求")).toBeVisible();
});
it.each(["disabled", "stale", "untracked"])("%s 状态不判为正在接收流量", (mode) => {
  const data = snapshot();
  if (mode === "disabled") data.enabled = false;
  if (mode === "stale") data.observed_at = new Date(Date.now() - 60_000).toISOString();
  if (mode === "untracked")
    data.accounts = [
      { account_id: "41", current_requests: 0, waiting_requests: 0, tracked: false },
    ];
  setup(data);
  expect(screen.getByRole("button", { name: "选择实时流量（0）" })).toBeDisabled();
  expect(screen.queryByLabelText("账号 41：真实请求 · 2")).not.toBeInTheDocument();
});
it("刷新失败后旧计数不参与选择并显示读取失败", async () => {
  vi.spyOn(api, "accountTraffic").mockRejectedValue(new Error("monitor unavailable"));
  const view = setup(snapshot());
  await act(async () => {
    await view.client.invalidateQueries({ queryKey: ["accounts", "live-traffic"] });
  });
  expect(await screen.findByLabelText("账号 41：流量读取失败")).toBeVisible();
  expect(screen.getByRole("button", { name: "选择实时流量（0）" })).toBeDisabled();
});

it("当前范围有超过二十个活跃账号时键盘选择只提交前二十个稳定 ID", async () => {
  const user = userEvent.setup();
  const ids = Array.from({ length: 21 }, (_, index) => String(index + 1));
  const view = setup(
    {
      ...snapshot(),
      accounts: ids.map((account_id) => ({
        account_id,
        current_requests: 1,
        waiting_requests: 0,
        tracked: true,
      })),
    },
    ids,
  );
  screen.getByRole("button", { name: "选择实时流量（21）" }).focus();
  await user.keyboard("{Enter}");
  expect(view.select).toHaveBeenCalledWith(ids.slice(0, 20));
});

it("未返回账号计数时显示未知且不能选择", () => {
  setup({ ...snapshot(), accounts: [] });
  expect(screen.getByLabelText("账号 41：流量未知")).toBeVisible();
  expect(screen.getByRole("button", { name: "选择实时流量（0）" })).toBeDisabled();
});
