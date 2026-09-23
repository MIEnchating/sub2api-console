import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { api, type AccountModelSyncPreview, type Task } from "@/api";
import { AccountModelSyncDialog } from "../account-model-sync-dialog";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it.each(["discovery", "apply"] as const)(
  "%s 任务状态读取失败后可以重试读取并关闭弹窗",
  async (phase) => {
    const discovery: Task = {
      id: "discovery",
      skill: "account-model-sync",
      operation: "account-model-discovery",
      status: "succeeded",
      progress: 100,
      message: "模型发现完成",
      result: { items: [{ account_id: "41", account_name: "账号 A", status: "succeeded" }] },
      created_at: "2026-09-14T00:00:00Z",
      updated_at: "2026-09-14T00:00:01Z",
    };
    const preview: AccountModelSyncPreview = {
      account_count: 1,
      accounts_with_catalog: 1,
      blocked_patterns: [],
      blocked_models: [],
      models: [{ model: "model-a", account_count: 1 }],
      accounts: [
        {
          account_id: "41",
          account_name: "账号 A",
          models: ["model-a"],
          enabled_models: ["model-a"],
          probe_model: "model-a",
        },
      ],
      fingerprint: "catalog",
    };
    vi.spyOn(api, "dictionaries").mockResolvedValue({ items: [] });
    vi.spyOn(api, "discoverAccountModels").mockResolvedValue(discovery);
    vi.spyOn(api, "previewAccountModels").mockResolvedValue(preview);
    vi.spyOn(api, "applyAccountModels").mockResolvedValue({ ...discovery, id: "apply" });
    const load = vi.spyOn(api, "task").mockImplementation(async (id) => {
      if (id === phase) throw new Error("状态读取失败");
      return discovery;
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const close = vi.fn();
    render(
      <QueryClientProvider client={client}>
        <AccountModelSyncDialog
          open
          accountIds={["41"]}
          accountPlatforms={new Map()}
          accountGroups={new Map()}
          onOpenChange={close}
          onCompleted={() => undefined}
        />
      </QueryClientProvider>,
    );
    await userEvent.click(screen.getByRole("button", { name: "选择全部分组" }));
    await userEvent.click(screen.getByRole("button", { name: "开始同步" }));
    if (phase === "apply") {
      await userEvent.click(await screen.findByRole("button", { name: "同步 1 个账号" }));
    }
    const queryKey = [`account-model-${phase}`, phase];
    await waitFor(() => expect(client.getQueryState(queryKey)?.status).toBe("error"));

    for (const closeButton of screen.getAllByRole("button", { name: "关闭" })) {
      expect(closeButton).toBeEnabled();
    }
    load.mockResolvedValue({ ...discovery, id: phase, status: "cancelled", result: {} });
    await userEvent.click(screen.getByRole("button", { name: "重新读取" }));
    await waitFor(() => expect(client.getQueryState(queryKey)?.status).toBe("success"));
    await userEvent.keyboard("{Escape}");
    expect(close).toHaveBeenCalledWith(false);
    cleanup();
    client.clear();
  },
);
