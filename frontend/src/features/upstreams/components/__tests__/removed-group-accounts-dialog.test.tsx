import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api, type UpstreamGroupChange } from "@/api";
import { RemovedGroupAccountsDialog } from "../removed-group-accounts-dialog";

const change: UpstreamGroupChange = {
  id: 2,
  upstream_id: "up-1",
  group_id: "8",
  group_name: "旧组",
  change_type: "removed",
  changed_at: "2026-09-02T00:00:00Z",
};

function renderDialog(expectedAccountIDs = ["11"]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onFinished = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <RemovedGroupAccountsDialog
        change={change}
        expectedAccountIDs={expectedAccountIDs}
        onClose={vi.fn()}
        onFinished={onFinished}
      />
    </QueryClientProvider>,
  );
  return onFinished;
}

afterEach(() => vi.restoreAllMocks());

it("分组绑定账号在打开前变化时停止删除并要求重新查看", async () => {
  vi.spyOn(api, "upstreamGroupBindingAudit").mockResolvedValue({
    items: [
      {
        upstream_id: "up-1",
        host: "api.example.test",
        group_id: "8",
        group_name: "旧组",
        account_count: 1,
        accounts: [{ id: "12", name: "另一账号" }],
        status: "missing",
        reason: null,
      },
    ],
    total_bindings: 1,
    present: 0,
    missing: 1,
    unknown: 0,
  });
  const preview = vi.spyOn(api, "accountDeleteBatchPreview");
  renderDialog();
  expect(await screen.findByText("分组绑定已变化，请关闭后重新查看统计变化。")).toBeVisible();
  expect(screen.queryByRole("button", { name: /确认删除/ })).not.toBeInTheDocument();
  expect(preview).not.toHaveBeenCalled();
});

it("确认删除时重新核对稳定分组和账号影响范围后创建删除任务", async () => {
  const audit = {
    items: [
      {
        upstream_id: "up-1",
        host: "api.example.test",
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
    upstream_key_count: 1,
    accounts: [
      {
        account_id: "11",
        account_name: "账号甲",
        groups: ["本地组"],
        management_base_url: "https://management.example.test",
        binding: {
          id: 4,
          upstream_id: "up-1",
          upstream_host: "api.example.test",
          auth_host: "api.example.test",
          upstream_key_id: "99",
          upstream_key_name: "Key 甲",
        },
      },
    ],
  };
  vi.spyOn(api, "upstreamGroupBindingAudit").mockResolvedValue(audit);
  const previewRequest = vi.spyOn(api, "accountDeleteBatchPreview").mockResolvedValue(preview);
  const deleteRequest = vi.spyOn(api, "deleteAccounts").mockResolvedValue({
    id: "task-1",
    skill: "account",
    operation: "account-delete-batch",
    status: "queued",
    progress: 0,
    message: "已创建",
    result: {},
    created_at: "2026-09-02T00:00:00Z",
    updated_at: "2026-09-02T00:00:00Z",
  });
  vi.spyOn(api, "task").mockResolvedValue({
    id: "task-1",
    skill: "account",
    operation: "account-delete-batch",
    status: "succeeded",
    progress: 100,
    message: "已删除 1 个账号",
    result: {},
    created_at: "2026-09-02T00:00:00Z",
    updated_at: "2026-09-02T00:01:00Z",
  });
  const onFinished = renderDialog();
  const dialog = await screen.findByRole("dialog", { name: "删除「旧组」的绑定账号" });
  expect(await within(dialog).findByText("账号甲（ID 11）")).toBeVisible();
  expect(within(dialog).getByText(/连带删除上游 Key：Key 甲/)).toBeVisible();
  await userEvent.setup().click(within(dialog).getByRole("button", { name: "确认删除 1 个账号" }));
  expect(await within(dialog).findByRole("status")).toHaveTextContent("已删除 1 个账号");
  expect(previewRequest).toHaveBeenCalledTimes(2);
  expect(deleteRequest).toHaveBeenCalledWith(preview);
  expect(onFinished).toHaveBeenCalledOnce();
});

it("确认时分组绑定发生变化则不调用删除接口", async () => {
  const audit = {
    items: [
      {
        upstream_id: "up-1",
        host: "api.example.test",
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
  const auditRequest = vi
    .spyOn(api, "upstreamGroupBindingAudit")
    .mockResolvedValueOnce(audit)
    .mockResolvedValueOnce({
      ...audit,
      items: [{ ...audit.items[0], accounts: [{ id: "12", name: "账号乙" }] }],
    });
  vi.spyOn(api, "accountDeleteBatchPreview").mockResolvedValue({
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
  });
  const deleteRequest = vi.spyOn(api, "deleteAccounts");
  renderDialog();
  await screen.findByRole("button", { name: "确认删除 1 个账号" });
  await userEvent.setup().click(screen.getByRole("button", { name: "确认删除 1 个账号" }));
  await waitFor(() => expect(auditRequest.mock.calls.length).toBeGreaterThanOrEqual(2));
  expect(deleteRequest).not.toHaveBeenCalled();
});

it("删除分组绑定超过 50 个账号时本次仅预览前 50 个并提示继续处理", async () => {
  const accounts = Array.from({ length: 51 }, (_, index) => ({
    id: String(index + 1),
    name: `账号 ${index + 1}`,
  }));
  vi.spyOn(api, "upstreamGroupBindingAudit").mockResolvedValue({
    items: [
      {
        upstream_id: "up-1",
        host: "api.example.test",
        group_id: "8",
        group_name: "旧组",
        account_count: 51,
        accounts,
        status: "missing",
        reason: null,
      },
    ],
    total_bindings: 51,
    present: 0,
    missing: 1,
    unknown: 0,
  });
  const preview = vi.spyOn(api, "accountDeleteBatchPreview").mockResolvedValue({
    account_count: 50,
    upstream_key_count: 0,
    accounts: accounts.slice(0, 50).map((account) => ({
      account_id: account.id,
      account_name: account.name,
      groups: [],
      management_base_url: "https://management.example.test",
      binding: null,
    })),
  });
  renderDialog(accounts.map((account) => account.id));
  expect(await screen.findByText(/剩余账号请在本次完成后继续处理/)).toBeVisible();
  expect(screen.getByRole("button", { name: "确认删除 50 个账号" })).toBeVisible();
  expect(preview).toHaveBeenCalledWith(expect.arrayContaining(["1", "50"]));
  expect(preview.mock.calls[0]?.[0]).toHaveLength(50);
});
