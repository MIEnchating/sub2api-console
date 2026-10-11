import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { AccountGroupsPreview } from "@/api";
import { AccountGroupsDialog } from "../account-groups-dialog";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function Harness() {
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => setOpen(true)}>打开分组</button>
      {open && (
        <AccountGroupsDialog
          account={{ id: "41", name: "测试账号" }}
          onOpenChange={setOpen}
          onStarted={() => undefined}
        />
      )}
    </>
  );
}

it("关闭分组弹窗取消慢预览，重开显示新分组且不提交业务操作", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  const preview: AccountGroupsPreview = {
    account_id: "41",
    current_group_ids: ["7"],
    groups: [{ id: "7", name: "最新组" }],
    target_version: "version-2",
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
    .mockImplementation(async (input) =>
      Response.json(String(input).includes("dictionaries") ? { items: [] } : preview),
    );
  vi.stubGlobal("fetch", fetcher);
  render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await screen.findByRole("status", { name: "正在读取账号分组…" });
  try {
    await user.click(screen.getByRole("button", { name: "关闭" }));
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () =>
      release(Response.json({ ...preview, groups: [{ id: "7", name: "过期组" }] })),
    );
  }
  expect(client.getQueryData(["account-groups", "41"])).toBeUndefined();
  await user.click(screen.getByRole("button", { name: "打开分组" }));
  expect(await screen.findByText("当前分组：最新组")).toBeVisible();
  expect(screen.queryByText("当前分组：过期组")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认切换" })).toBeDisabled();
  expect(fetcher.mock.calls.every(([, init]) => !init?.method)).toBe(true);
});
