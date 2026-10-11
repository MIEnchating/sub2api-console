import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, type UpstreamGroupChange, type Task } from "@/api";
import { RemovedGroupAccountsDialog } from "../removed-group-accounts-dialog";

const change: UpstreamGroupChange = {
  id: 2,
  upstream_id: "up-1",
  group_id: "8",
  group_name: "旧组",
  change_type: "removed",
  changed_at: "2026-09-02T00:00:00Z",
};
const audit = {
  items: [
    {
      upstream_id: "up-1",
      host: "example.test",
      group_id: "8",
      group_name: "旧组",
      account_count: 1,
      accounts: [{ id: "11", name: "账号甲" }],
      status: "missing" as const,
      reason: null,
    },
  ],
  total_bindings: 1,
  present: 0,
  missing: 1,
  unknown: 0,
};
const preview = {
  account_count: 1,
  upstream_key_count: 0,
  accounts: [
    {
      account_id: "11",
      account_name: "账号甲",
      groups: [],
      management_base_url: "https://management.example.test",
      binding: null,
    },
  ],
};
const queued: Task = {
  id: "delete-1",
  skill: "account",
  operation: "account-delete-batch",
  status: "queued",
  progress: 0,
  message: "已创建删除任务",
  result: {},
  created_at: "2026-09-02T00:00:00Z",
  updated_at: "2026-09-02T00:00:00Z",
};
const clients: QueryClient[] = [];
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
});
function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: 2 } },
  });
  clients.push(client);
  const onFinished = vi.fn();
  function view(target = change) {
    return (
      <QueryClientProvider client={client}>
        <RemovedGroupAccountsDialog
          change={target}
          expectedAccountIDs={["11"]}
          onClose={vi.fn()}
          onFinished={onFinished}
        />
      </QueryClientProvider>
    );
  }
  return { ...render(view()), view, onFinished, client };
}

it.each(["audit", "preview"])("提交前 %s 复核等待时卸载，不继续发起账号删除", async (stage) => {
  let finish!: () => void;
  const gate = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const auditRequest = vi.spyOn(api, "upstreamGroupBindingAudit").mockResolvedValue(audit);
  const previewRequest = vi.spyOn(api, "accountDeleteBatchPreview").mockResolvedValue(preview);
  const deletion = vi.spyOn(api, "deleteAccounts").mockResolvedValue(queued);
  const page = setup();
  await screen.findByRole("button", { name: "确认删除 1 个账号" });
  if (stage === "audit")
    auditRequest.mockImplementationOnce(async () => {
      await gate;
      return audit;
    });
  else
    previewRequest.mockImplementationOnce(async () => {
      await gate;
      return preview;
    });
  fireEvent.click(screen.getByRole("button", { name: "确认删除 1 个账号" }));
  await waitFor(() =>
    expect(stage === "audit" ? auditRequest : previewRequest).toHaveBeenCalledTimes(2),
  );
  page.unmount();
  await act(async () => finish());
  expect(deletion).not.toHaveBeenCalled();
});

it("切换稳定分组后不继续显示或轮询旧分组删除任务", async () => {
  vi.spyOn(api, "upstreamGroupBindingAudit").mockResolvedValue(audit);
  vi.spyOn(api, "accountDeleteBatchPreview").mockResolvedValue(preview);
  vi.spyOn(api, "deleteAccounts").mockResolvedValue(queued);
  vi.spyOn(api, "task").mockResolvedValue(queued);
  const page = setup();
  fireEvent.click(await screen.findByRole("button", { name: "确认删除 1 个账号" }));
  await screen.findByText("已创建删除任务");
  page.rerender(page.view({ ...change, upstream_id: "up-2", group_name: "另一个分组" }));
  await waitFor(() => expect(screen.queryByText("已创建删除任务")).not.toBeInTheDocument());
  expect(page.onFinished).not.toHaveBeenCalled();
});

it("卸载后收到复核失败不展示旧错误也不自动重试", async () => {
  let fail!: (error: Error) => void;
  vi.spyOn(api, "upstreamGroupBindingAudit")
    .mockResolvedValueOnce(audit)
    .mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          fail = reject;
        }),
    );
  vi.spyOn(api, "accountDeleteBatchPreview").mockResolvedValue(preview);
  const errorToast = vi.spyOn(toast, "error");
  const page = setup();
  fireEvent.click(await screen.findByRole("button", { name: "确认删除 1 个账号" }));
  await waitFor(() => expect(fail).toBeDefined());
  page.unmount();
  await act(async () => fail(new Error("复核连接失败")));
  expect(errorToast).not.toHaveBeenCalled();
  const mutations = page.client.getMutationCache().getAll();
  expect(mutations[0]?.state.status).toBe("error");
});

it.each(["audit", "preview"])("关闭窗口时取消尚未完成的 %s 读取", async (stage) => {
  let signal: AbortSignal | undefined;
  vi.spyOn(api, "upstreamGroupBindingAudit").mockImplementation((value) => {
    if (stage === "audit") {
      signal = value;
      return new Promise(() => {});
    }
    return Promise.resolve(audit);
  });
  vi.spyOn(api, "accountDeleteBatchPreview").mockImplementation((_, value) => {
    signal = value;
    return new Promise(() => {});
  });
  const page = setup();
  await waitFor(() => expect(signal).toBeDefined());
  expect(signal?.aborted).toBe(false);
  page.unmount();
  expect(signal?.aborted).toBe(true);
});

it("删除请求已发送后卸载，迟到任务仍保存且不启动旧窗口轮询", async () => {
  vi.spyOn(api, "upstreamGroupBindingAudit").mockResolvedValue(audit);
  vi.spyOn(api, "accountDeleteBatchPreview").mockResolvedValue(preview);
  let finish!: (task: Task) => void;
  const deletion = vi.spyOn(api, "deleteAccounts").mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const taskRead = vi.spyOn(api, "task");
  const page = setup();
  page.client.setQueryData(["tasks"], []);
  fireEvent.click(await screen.findByRole("button", { name: "确认删除 1 个账号" }));
  await waitFor(() => expect(deletion).toHaveBeenCalledOnce());
  page.unmount();
  await act(async () => finish(queued));
  expect(page.client.getQueryData(["account-delete-batch", queued.id])).toEqual(queued);
  expect(page.client.getQueryState(["tasks"])?.isInvalidated).toBe(true);
  expect(taskRead).not.toHaveBeenCalled();
  expect(page.onFinished).not.toHaveBeenCalled();
});
