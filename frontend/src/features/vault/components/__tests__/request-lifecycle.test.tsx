import { onlineManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { VaultPage } from "../vault-page";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

function setup(
  options: {
    fail?: boolean;
    holdRefresh?: boolean;
    failRefresh?: boolean;
    holdWrite?: boolean;
  } = {},
) {
  let failRefresh = options.failRefresh;
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: 2, retryDelay: 0 },
    },
  });
  clients.push(client);
  const index = {
    auth_records: [],
    vault_entries: [
      {
        entry: "operator",
        hosts: ["api.example.test"],
        has_username: true,
        has_password: true,
        username_is_email: false,
        header_names: [],
      },
    ],
  };
  client.setQueryData(["auth-recovery-config"], index);
  const writes: string[] = [];
  let release: () => void = () => undefined;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST" || init?.method === "DELETE") {
        writes.push(init.method);
        if (options.holdWrite) await gate;
        if (options.fail)
          return Response.json(
            { error: { code: "write_failed", message: "写入失败" } },
            { status: 503 },
          );
        return Response.json({ entry: "operator", configured: true, deleted: true });
      }
      if (options.holdRefresh) await gate;
      if (failRefresh) return Response.json({ error: "索引读取失败" }, { status: 503 });
      return Response.json(index);
    }),
  );
  const view = render(
    <QueryClientProvider client={client}>
      <VaultPage />
    </QueryClientProvider>,
  );
  return {
    client,
    writes,
    release,
    unmount: view.unmount,
    recover: () => {
      failRefresh = false;
    },
  };
}

function submit(operation: "save" | "delete"): void {
  if (operation === "delete") {
    fireEvent.click(screen.getByRole("button", { name: "删除凭据" }));
    fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
    return;
  }
  fireEvent.click(screen.getByRole("button", { name: "编辑凭据" }));
  fireEvent.change(screen.getByLabelText("密码"), { target: { value: "replacement-secret" } });
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
}

it.each(["save", "delete"] as const)(
  "%s 失败时不继承全局自动重试，保留窗口供人工处理",
  async (operation) => {
    const state = setup({ fail: true });
    submit(operation);
    await waitFor(() => expect(state.client.isMutating()).toBe(0));
    expect(state.writes).toHaveLength(1);
    expect(
      screen.getByRole("dialog", { name: operation === "save" ? "编辑凭据" : "删除凭据" }),
    ).toBeVisible();
    if (operation === "save")
      expect(screen.getByLabelText("密码")).toHaveValue("replacement-secret");
  },
);

it.each(["save", "delete"] as const)(
  "%s 离线提交立即尝试，不在恢复网络后重放",
  async (operation) => {
    const state = setup({ fail: true });
    onlineManager.setOnline(false);
    submit(operation);
    await waitFor(() => expect(state.writes).toHaveLength(1));
    await waitFor(() => expect(state.client.isMutating()).toBe(0));
    await act(async () => {
      onlineManager.setOnline(true);
    });
    expect(state.writes).toHaveLength(1);
  },
);

it.each(["save", "delete"] as const)(
  "%s 成功后立即关闭弹窗，索引回读期间禁用旧数据操作",
  async (operation) => {
    const state = setup({ holdRefresh: true });
    submit(operation);
    await waitFor(() => expect(state.client.isFetching()).toBe(1));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "添加凭据" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "编辑凭据" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "删除凭据" })).toBeDisabled();
    await act(async () => {
      state.release();
    });
    await waitFor(() => expect(screen.getByRole("button", { name: "添加凭据" })).toBeEnabled());
  },
);

it("保存成功但索引回读失败时保留列表且禁止修改，人工刷新成功后恢复", async () => {
  const state = setup({ failRefresh: true });
  submit("save");
  await waitFor(() =>
    expect(state.client.getQueryState(["auth-recovery-config"])?.status).toBe("error"),
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.getByText("operator")).toBeVisible();
  expect(screen.getByRole("button", { name: "编辑凭据" })).toBeDisabled();
  state.recover();
  fireEvent.click(screen.getByRole("button", { name: "刷新密码箱" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "编辑凭据" })).toBeEnabled());
  expect(state.writes).toEqual(["POST"]);
});

it.each(["save", "delete"] as const)(
  "%s 请求未完成时阻止重复提交，离开页面后成功仍使索引失效",
  async (operation) => {
    const state = setup({ holdWrite: true });
    submit(operation);
    await waitFor(() => expect(state.writes).toHaveLength(1));
    const button = screen.getByRole("button", {
      name: operation === "save" ? "保存中..." : "删除中...",
    });
    expect(button).toBeDisabled();
    fireEvent.click(button);
    state.unmount();
    await act(async () => {
      state.release();
    });
    await waitFor(() => expect(state.client.isMutating()).toBe(0));
    expect(state.writes).toHaveLength(1);
    expect(state.client.getQueryState(["auth-recovery-config"])?.isInvalidated).toBe(true);
  },
);
