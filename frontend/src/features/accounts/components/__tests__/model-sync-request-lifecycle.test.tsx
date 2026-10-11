import { onlineManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { seedDictionaries } from "@/test/dictionary-fixtures";
import { AccountModelSyncDialog } from "../account-model-sync-dialog";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

function setup(
  options: {
    fail?: "discover" | "apply" | "preview";
    hold?: "preview" | "discover" | "apply";
  } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: 2, retryDelay: 0 } },
  });
  clients.push(client);
  seedDictionaries(client);
  const requests: string[] = [];
  let previewSignal: AbortSignal | null | undefined;
  let release: () => void = () => undefined;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let failure = options.fail;
  const task = {
    id: "discover",
    skill: "account-model-sync",
    operation: "account-model-discovery",
    status: "succeeded",
    progress: 100,
    message: "已完成",
    result: { items: [{ account_id: "41", status: "succeeded" }] },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      requests.push(path);
      const phase = path.split("/").at(-1);
      if (path.includes("/models/") && phase === "preview") previewSignal = init?.signal;
      if (path.includes("/models/") && phase === options.hold) await gate;
      if (path.includes("/models/") && phase === failure)
        return Response.json(
          { error: { code: "test_failure", message: "请求失败，请重试" } },
          { status: 503 },
        );
      if (path.endsWith("/models/preview"))
        return Response.json({
          account_count: 1,
          accounts_with_catalog: 1,
          blocked_models: [],
          blocked_patterns: [],
          fingerprint: "catalog",
          models: [],
          accounts: [
            {
              account_id: "41",
              account_name: "测试账号",
              models: ["model-a"],
              enabled_models: ["model-a"],
              probe_model: "",
            },
          ],
        });
      if (path.endsWith("/models/discover") || path.endsWith("/tasks/discover"))
        return Response.json(task);
      if (path.endsWith("/models/apply") || path.endsWith("/tasks/apply"))
        return Response.json({ ...task, id: "apply", operation: "account-model-apply" });
      if (path.includes("dictionaries")) return Response.json({ items: [] });
      throw new Error(`Unexpected request ${path}`);
    }),
  );
  const completed = vi.fn();
  const view = (open: boolean) => (
    <QueryClientProvider client={client}>
      <AccountModelSyncDialog
        open={open}
        accountIds={["41"]}
        accountPlatforms={new Map()}
        accountGroups={new Map()}
        onOpenChange={() => undefined}
        onCompleted={completed}
      />
    </QueryClientProvider>
  );
  const rendered = render(view(true));
  return {
    client,
    requests,
    completed,
    signal: () => previewSignal,
    release,
    recover: () => {
      failure = undefined;
    },
    failPreview: () => {
      failure = "preview";
    },
    close: () => rendered.rerender(view(false)),
  };
}

async function start(): Promise<void> {
  await userEvent.click(screen.getByRole("button", { name: "选择全部分组" }));
  await userEvent.click(screen.getByRole("button", { name: "开始同步" }));
}

it.each(["discover", "apply"] as const)("%s 创建失败时不继承全局自动重试", async (phase) => {
  const state = setup({ fail: phase });
  await start();
  if (phase === "apply")
    await userEvent.click(await screen.findByRole("button", { name: "同步 1 个账号" }));
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.requests.filter((path) => path.endsWith(`/models/${phase}`))).toHaveLength(1);
});

it("关闭正在读取预览的窗口时取消请求", async () => {
  const state = setup({ hold: "preview" });
  await start();
  await screen.findByText("正在生成模型同步预览");
  state.close();
  expect(state.signal()?.aborted).toBe(true);
  await act(async () => state.release());
});

it("预览读取失败后可以重试并恢复模型选择", async () => {
  const state = setup({ fail: "preview" });
  await start();
  await waitFor(() =>
    expect(state.client.getQueryState(["account-model-sync-preview", ["41"]])?.status).toBe(
      "error",
    ),
  );
  state.recover();
  await userEvent.click(screen.getByRole("button", { name: "重新读取" }));
  expect(await screen.findByRole("checkbox", { name: "同步模型 model-a" })).toBeChecked();
});

it.each(["discover", "apply"] as const)(
  "%s 创建后窗口卸载仍保存任务且不回调旧窗口",
  async (phase) => {
    const state = setup({ hold: phase });
    state.client.setQueryData(["tasks"], []);
    await start();
    if (phase === "apply")
      await userEvent.click(await screen.findByRole("button", { name: "同步 1 个账号" }));
    await waitFor(() =>
      expect(state.requests).toContain(`/api/management/accounts/models/${phase}`),
    );
    state.close();
    await act(async () => state.release());
    await waitFor(() => expect(state.client.isMutating()).toBe(0));
    const key = phase === "discover" ? "account-model-discovery" : "account-model-apply";
    expect(state.client.getQueryData([key, phase])).toMatchObject({ id: phase });
    expect(state.client.getQueryState(["tasks"])?.isInvalidated).toBe(true);
    expect(state.completed).not.toHaveBeenCalled();
  },
);

it.each(["discover", "apply"] as const)(
  "%s 离线提交立即尝试而不在联网后延迟执行",
  async (phase) => {
    const state = setup({ fail: phase });
    if (phase === "apply") {
      await start();
      await screen.findByRole("button", { name: "同步 1 个账号" });
    }
    onlineManager.setOnline(false);
    if (phase === "discover") await start();
    else await userEvent.click(screen.getByRole("button", { name: "同步 1 个账号" }));
    await waitFor(() =>
      expect(state.requests.filter((path) => path.endsWith(`/models/${phase}`))).toHaveLength(1),
    );
    await waitFor(() => expect(state.client.isMutating()).toBe(0));
  },
);

it("后台预览刷新失败时保留已有模型但禁止提交，重试成功后恢复", async () => {
  const state = setup();
  await start();
  const submit = await screen.findByRole("button", { name: "同步 1 个账号" });
  state.failPreview();
  await act(async () => {
    await state.client.invalidateQueries({ queryKey: ["account-model-sync-preview"] });
  });
  expect(screen.getByRole("checkbox", { name: "同步模型 model-a" })).toBeChecked();
  await waitFor(() => expect(submit).toBeDisabled());
  state.recover();
  await userEvent.click(screen.getByRole("button", { name: "重新读取" }));
  await waitFor(() => expect(submit).toBeEnabled());
});
