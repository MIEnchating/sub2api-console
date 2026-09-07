import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { api, type AccountDeletePreview, type Task, type UpstreamGroup } from "@/api";
import { UpstreamAccounts } from "../upstream-edit-dialog";

const preview: AccountDeletePreview = {
  account_id: "41",
  account_name: "Codex 主账号",
  groups: ["codex-local"],
  management_base_url: "https://management.example.test",
  binding: {
    id: 1,
    upstream_id: "up_example",
    upstream_host: "api.example.test",
    auth_host: "api.example.test",
    upstream_key_id: "key-41",
    upstream_key_name: "主 Key",
  },
};

const group: UpstreamGroup = {
  upstream_id: "up_example",
  host: "api.example.test",
  group_id: "codex",
  name: "codex",
  description: "Codex pool",
  platform: "openai",
  status: "active",
  raw_rate: "0.2",
  effective_rate: "0.2",
  recharge_rate: "1",
  bound: true,
  bound_accounts: [
    {
      binding_id: 1,
      account_id: "41",
      account_name: "Codex 主账号",
      account_exists: true,
      binding_status: "active",
      local_group: "codex-local",
      upstream_key_id: "key-41",
      upstream_key_name: "主 Key",
    },
  ],
  key_present: true,
  bindable: false,
  unavailable_reason: null,
};

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: "task-41",
    skill: "test",
    operation: "active-probe",
    status: "queued",
    progress: 0,
    message: "已排队",
    result: {},
    created_at: "2026-09-07T00:00:00Z",
    updated_at: "2026-09-07T00:00:00Z",
    ...overrides,
  };
}

function renderAccounts(onChanged = vi.fn(), groups: UpstreamGroup[] = [group]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={queryClient}>
      <UpstreamAccounts groups={groups} interactive onChanged={onChanged} />
    </QueryClientProvider>,
  );
  return onChanged;
}

beforeAll(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.restoreAllMocks());
afterAll(() => vi.unstubAllGlobals());

describe("编辑上游账号操作", () => {
  it("探活使用与账号管理相同的稳定账号 ID 接口", async () => {
    const runProbe = vi.spyOn(api, "runActiveProbe").mockResolvedValue(task());
    vi.spyOn(api, "task").mockResolvedValue(
      task({ status: "succeeded", progress: 100, message: "探活完成" }),
    );
    const onChanged = renderAccounts();

    fireEvent.click(screen.getByRole("button", { name: "探活测试" }));

    expect(runProbe).toHaveBeenCalledWith({ account_id: "41" });
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
  });

  it("删除先读取账号管理的删除范围并提交同一稳定 ID 预览", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "accountDeletePreview").mockResolvedValue(preview);
    const deleteAccount = vi
      .spyOn(api, "deleteAccount")
      .mockResolvedValue(task({ operation: "account-delete" }));
    vi.spyOn(api, "task").mockResolvedValue(
      task({
        operation: "account-delete",
        status: "succeeded",
        progress: 100,
        result: { account_id: "41", local_projection_deleted: true },
      }),
    );
    renderAccounts();

    fireEvent.click(screen.getByRole("button", { name: "删除账号及上游 Key" }));
    expect(await screen.findByText("key-41")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "确认删除" }));

    expect(deleteAccount).toHaveBeenCalledWith(preview);
  });

  it("本地账号不存在时禁用探活和删除", () => {
    const missingGroup = {
      ...group,
      bound_accounts: [{ ...group.bound_accounts[0]!, account_exists: false }],
    };
    renderAccounts(vi.fn(), [missingGroup]);

    expect(screen.getByRole("button", { name: "探活测试" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "删除账号及上游 Key" })).toBeDisabled();
  });
});
