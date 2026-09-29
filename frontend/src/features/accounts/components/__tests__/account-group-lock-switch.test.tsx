import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { account } from "../../__tests__/fixtures";
import type { AccountStatus } from "@/api";
import { AccountGroupLockSwitch } from "../account-group-lock-switch";

function AccountListLock(props: { fallback: AccountStatus }) {
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: async () => [props.fallback],
    enabled: false,
  });
  return <AccountGroupLockSwitch account={accounts.data?.[0] ?? props.fallback} />;
}

function setup(locked = false, groups = ["原分组"], manual: number | null = null) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const row = { ...account, groups, groups_locked: locked, manual_priority: manual };
  client.setQueryData(["accounts"], [row]);
  const view = render(
    <QueryClientProvider client={client}>
      <AccountListLock fallback={row} />
    </QueryClientProvider>,
  );
  return { client, row, ...view };
}

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("开启时发送账号 ID 和布尔值，并在确认后更新缓存状态", async () => {
  const fetch = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ groups_locked: true }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  const f = setup(false, ["原分组", "备用组"]);
  const control = screen.getByRole("switch", { name: `锁定分组：${f.row.name}` });
  expect(control).toHaveAttribute("aria-checked", "false");
  control.focus();
  await user.keyboard(" ");
  await waitFor(() => expect(control).toHaveAttribute("aria-checked", "true"));
  expect(fetch).toHaveBeenCalledWith(
    expect.stringContaining(`/api/accounts/${f.row.id}/group-lock`),
    expect.objectContaining({ method: "PUT", body: JSON.stringify({ groups_locked: true }) }),
  );
  expect(f.client.getQueryData<(typeof f.row)[]>(["accounts"])?.[0].groups_locked).toBe(true);
  f.client.clear();
});

it("手动控制账号也能关闭独立分组锁定", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ groups_locked: false }), {
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
  const user = userEvent.setup();
  const f = setup(true, ["原分组"], 2);
  const control = screen.getByRole("switch", { name: `锁定分组：${f.row.name}` });
  await user.click(control);
  await waitFor(() => expect(control).toHaveAttribute("aria-checked", "false"));
  expect(f.client.getQueryData<(typeof f.row)[]>(["accounts"])?.[0].manual_priority).toBe(2);
  f.client.clear();
});

it("未分组账号无法开启，但已有锁定仍允许关闭", async () => {
  const f = setup(false, []);
  expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true");
  f.client.setQueryData(["accounts"], [{ ...f.row, groups_locked: true }]);
  await waitFor(() =>
    expect(screen.getByRole("switch")).not.toHaveAttribute("aria-disabled", "true"),
  );
  f.client.clear();
});

it("请求失败时保留原锁定状态", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { message: "锁定失败" } }), {
        status: 409,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
  const user = userEvent.setup();
  const f = setup();
  const control = screen.getByRole("switch");
  await user.click(control);
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  expect(control).toHaveAttribute("aria-checked", "false");
  expect(f.client.getQueryData<(typeof f.row)[]>(["accounts"])?.[0].groups_locked).toBe(false);
  f.client.clear();
});
