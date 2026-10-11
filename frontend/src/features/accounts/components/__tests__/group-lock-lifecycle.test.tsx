import { useState, type ReactElement } from "react";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, type AccountStatus } from "@/api";
import { account } from "../../__tests__/fixtures";
import { AccountGroupLockSwitch } from "../account-group-lock-switch";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function Host(): ReactElement {
  const [id, setID] = useState("41");
  const accounts = useQuery<AccountStatus[]>({
    queryKey: ["accounts"],
    queryFn: ({ signal }) => api.accounts(signal),
    enabled: false,
  });
  const selected = accounts.data?.find((item) => item.id === id);
  return (
    <>
      <button onClick={() => setID("41")}>查看账号 A</button>
      <button onClick={() => setID("42")}>查看账号 B</button>
      {selected && <AccountGroupLockSwitch account={selected} />}
    </>
  );
}

it("账号A锁定提交未返回时切换到账号B，迟到成功只更新A的列表和详情", async () => {
  const user = userEvent.setup();
  let resolveResponse!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveResponse = resolve;
        }),
    ),
  );
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  clients.push(client);
  const rows = [
    { ...account, id: "41", name: "账号 A", groups_locked: false, groups: ["分组"] },
    { ...account, id: "42", name: "账号 B", groups_locked: false, groups: ["分组"] },
  ];
  client.setQueryData(["accounts"], rows);
  for (const row of rows) client.setQueryData(["account-detail", row.id], row);
  render(
    <QueryClientProvider client={client}>
      <Host />
    </QueryClientProvider>,
  );

  await user.click(screen.getByRole("switch", { name: "锁定分组：账号 A" }));
  await waitFor(() => expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true"));
  await user.click(screen.getByRole("button", { name: "查看账号 B" }));
  await act(async () => resolveResponse(Response.json({ groups_locked: true })));
  const other = screen.getByRole("switch", { name: "锁定分组：账号 B" });
  await waitFor(() => expect(other).not.toHaveAttribute("aria-disabled", "true"));
  expect(other).toHaveAttribute("aria-checked", "false");
  expect(client.getQueryData<AccountStatus>(["account-detail", "42"])?.groups_locked).toBe(false);
  await user.click(screen.getByRole("button", { name: "查看账号 A" }));
  expect(screen.getByRole("switch", { name: "锁定分组：账号 A" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  expect(client.getQueryData<AccountStatus>(["account-detail", "41"])?.groups_locked).toBe(true);
});
