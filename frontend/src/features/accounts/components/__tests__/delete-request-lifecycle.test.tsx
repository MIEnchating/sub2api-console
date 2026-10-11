import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type AccountDeletePreview } from "@/api";
import { AccountDeleteDialog } from "../account-delete-dialog";

const preview: AccountDeletePreview = {
  account_id: "41",
  account_name: "最新账号",
  groups: [],
  management_base_url: "https://management.test",
  binding: null,
};
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function SharedPreview() {
  const query = useQuery({
    queryKey: ["account-delete-preview", "41"],
    queryFn: () => api.accountDeletePreview("41"),
  });
  return <output aria-label="共享删除预览">{query.data?.account_name}</output>;
}

function Harness(props: { shared: boolean }) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => setOpen(true)}>打开删除范围</button>
      <AccountDeleteDialog
        accountId="41"
        open={open}
        onOpenChange={setOpen}
        pending={false}
        activeAction={null}
        task={undefined}
        taskError={null}
        onConfirm={() => undefined}
      />
      {props.shared && <SharedPreview />}
    </>
  );
}

function setup(shared = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 15_000 } },
  });
  clients.push(client);
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
    .mockImplementation(async () => Response.json(preview));
  vi.stubGlobal("fetch", fetcher);
  const view = render(
    <QueryClientProvider client={client}>
      <Harness shared={shared} />
    </QueryClientProvider>,
  );
  return {
    ...view,
    client,
    fetcher,
    signal: () => signal,
    release: (response: Response) => release(response),
  };
}

it("关闭仍挂载的删除弹窗取消孤立预览，迟到响应不入缓存且重开读取最新范围", async () => {
  const view = setup();
  const user = userEvent.setup();
  await screen.findByRole("status", { name: "正在读取账号删除范围" });
  expect(screen.queryByRole("button", { name: "确认删除" })).not.toBeInTheDocument();
  try {
    await user.click(screen.getByRole("button", { name: "关闭" }));
    expect(view.signal()?.aborted).toBe(true);
  } finally {
    await act(async () => view.release(Response.json({ ...preview, account_name: "过期账号" })));
  }
  expect(view.client.getQueryData(["account-delete-preview", "41"])).toBeUndefined();
  await user.click(screen.getByRole("button", { name: "打开删除范围" }));
  expect(await screen.findByText("最新账号（ID 41）")).toBeVisible();
  expect(screen.queryByText("过期账号（ID 41）")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认删除" })).toBeEnabled();
  expect(
    view.fetcher.mock.calls.every(
      ([input, init]) => String(input) === "/api/accounts/41/delete-preview" && !init?.method,
    ),
  ).toBe(true);
});

it("关闭删除弹窗时另一观察者仍使用预览则保留共享读取", async () => {
  const view = setup(true);
  const user = userEvent.setup();
  await screen.findByRole("status", { name: "正在读取账号删除范围" });
  try {
    await user.click(screen.getByRole("button", { name: "关闭" }));
    expect(view.signal()?.aborted).toBe(false);
  } finally {
    await act(async () => view.release(Response.json(preview)));
  }
  await waitFor(() => expect(screen.getByLabelText("共享删除预览")).toHaveTextContent("最新账号"));
  expect(view.fetcher).toHaveBeenCalledTimes(1);
});

it("刚读取过删除范围又重开时重新核对，读取期间保留旧范围但禁用确认", async () => {
  const view = setup();
  const user = userEvent.setup();
  await act(async () => view.release(Response.json({ ...preview, account_name: "原账号" })));
  expect(await screen.findByText("原账号（ID 41）")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "取消" }));
  let release!: (response: Response) => void;
  view.fetcher.mockImplementation(
    () =>
      new Promise<Response>((resolve) => {
        release = resolve;
      }),
  );
  await user.click(screen.getByRole("button", { name: "打开删除范围" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(2));
  expect(screen.getByText("原账号（ID 41）")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认删除" })).toBeDisabled();
  await act(async () => release(Response.json(preview)));
  expect(await screen.findByText("最新账号（ID 41）")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认删除" })).toBeEnabled();
});
