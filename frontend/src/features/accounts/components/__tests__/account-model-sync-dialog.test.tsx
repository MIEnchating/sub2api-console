import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, ApiError, type AccountModelSyncPreview, type Task } from "@/api";
import {
  AccountModelSyncDialog,
  accountModelSyncDialogLayout,
  availableUnifiedProbeModels,
  modelSyncProbeItems,
  modelSyncTaskItems,
  preferredAccountProbeModel,
  selectedAccountModels,
  successfulModelSyncAccountIDs,
} from "../account-model-sync-dialog";

function task(items: unknown[]): Task {
  return {
    id: "model-discovery",
    skill: "sub2api-account-model-sync",
    operation: "account-model-discovery",
    status: "partial",
    progress: 100,
    message: "账号模型发现完成",
    result: { items },
    created_at: "2026-09-05T00:00:00Z",
    updated_at: "2026-09-05T00:01:00Z",
  };
}

describe("账号批量模型同步", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("只把发现成功的账号带入模型同步预览", () => {
    const discovery = task([
      { account_id: "41", account_name: "账号 A", status: "succeeded" },
      { account_id: "42", account_name: "账号 B", status: "failed", error: "上游超时" },
    ]);

    expect(successfulModelSyncAccountIDs(discovery)).toEqual(["41"]);
    expect(modelSyncTaskItems(discovery)[1]).toMatchObject({
      accountId: "42",
      status: "failed",
      error: "上游超时",
    });
  });

  it("默认同步新发现模型并自动应用全局屏蔽规则", () => {
    const preview: AccountModelSyncPreview = {
      account_count: 2,
      accounts_with_catalog: 2,
      blocked_patterns: ["model-d", "removed-model"],
      blocked_models: ["model-d"],
      models: [
        { model: "model-a", account_count: 2 },
        { model: "model-b", account_count: 1 },
        { model: "model-d", account_count: 2 },
      ],
      accounts: [
        {
          account_id: "41",
          account_name: "账号 A",
          platform: "anthropic",
          models: ["model-a", "model-d"],
          probe_model: "model-d",
        },
        {
          account_id: "42",
          account_name: "账号 B",
          platform: "openai",
          models: ["model-a", "model-b", "model-d"],
          probe_model: "model-b",
        },
      ],
      fingerprint: "fingerprint",
    };

    expect(preferredAccountProbeModel(preview.accounts[0], new Set(["model-d"]))).toBe("model-a");
    const selections = selectedAccountModels(
      preview,
      new Map([
        ["41", "anthropic"],
        ["42", "openai"],
      ]),
      new Map(),
      new Map(),
    );
    expect(selections).toEqual([
      { account_id: "41", models: ["model-a"] },
      { account_id: "42", models: ["model-a", "model-b"] },
    ]);
    expect(availableUnifiedProbeModels(selections)).toEqual(["model-a", "model-b"]);
  });

  it("账号属于多个分组时合并各分组选择，避免一个分组误删另一个分组需要的模型", () => {
    const preview: AccountModelSyncPreview = {
      account_count: 1,
      accounts_with_catalog: 1,
      blocked_patterns: [],
      blocked_models: [],
      models: [{ model: "shared-model", account_count: 1 }],
      accounts: [
        {
          account_id: "41",
          account_name: "共享账号",
          platform: "openai",
          models: ["shared-model"],
          probe_model: "shared-model",
        },
      ],
      fingerprint: "fingerprint",
    };
    const platforms = new Map([["41", "openai"]]);
    const groups = new Map([["41", ["分组 A", "分组 B"]]]);
    const onlyAExcluded = new Map([["openai\u0000分组 a", new Set(["shared-model"])]]);

    expect(selectedAccountModels(preview, platforms, groups, onlyAExcluded)[0]?.models).toEqual([
      "shared-model",
    ]);
    const bothExcluded = new Map([
      ["openai\u0000分组 a", new Set(["shared-model"])],
      ["openai\u0000分组 b", new Set(["shared-model"])],
    ]);
    expect(selectedAccountModels(preview, platforms, groups, bothExcluded)[0]?.models).toEqual([]);
  });

  it("创建任务时保持紧凑，结果出现后才扩大并固定页脚", () => {
    const pendingLayout = accountModelSyncDialogLayout("pending");
    const selectionLayout = accountModelSyncDialogLayout("selection");
    const resultLayout = accountModelSyncDialogLayout("result");

    expect(pendingLayout.width).toBe("progress");
    expect(pendingLayout.height).toBe("content");
    expect(pendingLayout.content).not.toContain("h-[min(52rem,calc(100svh-2rem))]");
    expect(selectionLayout.width).toBe("table");
    expect(selectionLayout.height).toBe("content");
    expect(selectionLayout.content).not.toContain("h-[min(52rem,calc(100svh-2rem))]");
    expect(resultLayout.width).toBe("table");
    expect(resultLayout.height).toBe("adaptive");
    expect(resultLayout.content).toContain("h-[min(52rem,calc(100svh-2rem))]");
    expect(resultLayout.content).toContain("grid-rows-[auto_minmax(0,1fr)_auto]");
    expect(resultLayout.content).toContain("overflow-hidden");
  });

  it("保留每个账号单独探测所用模型和失败原因", () => {
    const result = task([{ account_id: "41", account_name: "账号 A", status: "succeeded" }]);
    result.result.probe = {
      passed: 0,
      failed: 1,
      results: [
        {
          account_id: "41",
          group_name: "默认分组",
          result: "失败",
          request_model: "model-a",
          failure_reason: "上游返回 403",
        },
      ],
    };

    expect(modelSyncProbeItems(result)).toEqual([
      {
        accountId: "41",
        groupName: "默认分组",
        result: "失败",
        requestModel: "model-a",
        error: "上游返回 403",
      },
    ]);
  });

  it("默认状态无需逐账号配置即可直接同步", async () => {
    const discovery = task([{ account_id: "41", account_name: "账号 A", status: "succeeded" }]);
    const applyTask = {
      ...task([{ account_id: "41", account_name: "账号 A", status: "succeeded" }]),
      id: "model-apply",
      operation: "account-model-apply",
    };
    const preview: AccountModelSyncPreview = {
      account_count: 1,
      accounts_with_catalog: 1,
      blocked_patterns: ["model-b"],
      blocked_models: ["model-b"],
      models: [
        { model: "model-a", account_count: 1 },
        { model: "model-b", account_count: 1 },
      ],
      accounts: [
        {
          account_id: "41",
          account_name: "账号 A",
          models: ["model-a", "model-b"],
          probe_model: "model-b",
        },
      ],
      fingerprint: "catalog-fingerprint",
    };
    vi.spyOn(api, "discoverAccountModels").mockResolvedValue(discovery);
    vi.spyOn(api, "task").mockImplementation(async (taskId) =>
      taskId === applyTask.id ? applyTask : discovery,
    );
    vi.spyOn(api, "previewAccountModels").mockResolvedValue(preview);
    const apply = vi.spyOn(api, "applyAccountModels").mockResolvedValue(applyTask);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <AccountModelSyncDialog
          open
          accountIds={["41"]}
          accountPlatforms={new Map([["41", "openai"]])}
          accountGroups={new Map([["41", ["基础组"]]])}
          onOpenChange={() => undefined}
          onCompleted={() => undefined}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("tab", { name: /OpenAI/ })).toBeVisible();
    expect(screen.getByRole("tab", { name: /基础组/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("dialog").className).toContain("w-[min(90rem,calc(100vw-2rem))]");
    expect(screen.getByRole("dialog").className).not.toContain("h-[min(52rem,calc(100svh-2rem))]");
    expect(screen.queryByText(/准备同步 1 个账号/)).not.toBeInTheDocument();
    expect(screen.queryByText(/当前选择将写入/)).not.toBeInTheDocument();
    expect(screen.queryByText(/每个账号只写入自己实际返回且已勾选的模型/)).not.toBeInTheDocument();
    expect(screen.getByText("统一探活模型")).toBeVisible();
    expect(screen.getByRole("button", { name: "统一探活模型说明" })).toBeVisible();
    expect(
      screen.queryByText("最多选择 20 个；同步完成后依次验证，不支持的账号自动跳过。"),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "统一探活模型" })).toHaveTextContent("model-a");
    expect(screen.getByRole("heading", { name: "选择要同步的模型" })).toBeVisible();
    expect(screen.getByRole("checkbox", { name: "同步模型 model-a" })).toBeChecked();
    expect(screen.queryByRole("checkbox", { name: "同步模型 model-b" })).toBeNull();
    expect(screen.queryByText("已被全局规则屏蔽")).toBeNull();
    expect(screen.getByText("已选 1/1 个模型")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "同步 1 个账号" }));
    await waitFor(() =>
      expect(apply).toHaveBeenCalledWith(
        [{ account_id: "41", models: ["model-a"] }],
        "catalog-fingerprint",
        ["model-a"],
      ),
    );
  });

  it("按平台分类选择同步模型并统一设置探活模型", async () => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    const discovery = task([
      { account_id: "41", account_name: "账号 A", status: "succeeded" },
      { account_id: "42", account_name: "账号 B", status: "succeeded" },
      {
        account_id: "43",
        account_name: "失败账号",
        status: "failed",
        error: "管理 API 返回 HTTP 502：Cloudflare 响应不完整",
      },
    ]);
    const applyTask = {
      ...task([
        { account_id: "41", account_name: "账号 A", status: "succeeded" },
        { account_id: "42", account_name: "账号 B", status: "succeeded" },
      ]),
      id: "model-apply",
      operation: "account-model-apply",
    };
    applyTask.result.probe = {
      passed: 0,
      failed: 1,
      skipped: 1,
      results: [
        {
          account_id: "41",
          group_name: "默认分组",
          result: "失败",
          request_model: "model-a",
          failure_reason:
            'API returned 503: {"error":{"message":"Service temporarily unavailable","type":"api_error"}}',
        },
        {
          account_id: "42",
          group_name: "默认分组",
          result: "跳过",
          request_model: "model-a",
          failure_reason: "所选实际验证模型不在该账号的已启用模型中",
        },
      ],
    };
    const preview: AccountModelSyncPreview = {
      account_count: 2,
      accounts_with_catalog: 2,
      blocked_patterns: ["removed-model"],
      blocked_models: [],
      models: [
        { model: "model-a", account_count: 2 },
        { model: "model-b", account_count: 1 },
        { model: "model-e", account_count: 1 },
      ],
      accounts: [
        {
          account_id: "41",
          account_name: "账号 A",
          platform: "anthropic",
          models: ["model-a", "model-e"],
          probe_model: "",
        },
        {
          account_id: "42",
          account_name: "账号 B",
          platform: "openai",
          models: ["model-a", "model-b", "model-e"],
          probe_model: "model-b",
        },
      ],
      fingerprint: "catalog-fingerprint",
    };
    vi.spyOn(api, "discoverAccountModels").mockResolvedValue(discovery);
    vi.spyOn(api, "task").mockImplementation(async (taskId) =>
      taskId === applyTask.id ? applyTask : discovery,
    );
    vi.spyOn(api, "previewAccountModels").mockResolvedValue(preview);
    const apply = vi.spyOn(api, "applyAccountModels").mockResolvedValue(applyTask);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <AccountModelSyncDialog
          open
          accountIds={["41", "42"]}
          accountPlatforms={
            new Map([
              ["41", "anthropic"],
              ["42", "openai"],
            ])
          }
          accountGroups={
            new Map([
              ["41", ["Claude 组"]],
              ["42", ["OpenAI 组"]],
            ])
          }
          onOpenChange={() => undefined}
          onCompleted={() => undefined}
        />
      </QueryClientProvider>,
    );

    const anthropicTab = await screen.findByRole("tab", { name: /Anthropic/ });
    const openaiTab = screen.getByRole("tab", { name: /OpenAI/ });
    expect(anthropicTab).toHaveAttribute("aria-selected", "true");
    expect(openaiTab).toHaveAttribute("aria-selected", "false");
    expect(screen.getByRole("combobox", { name: "统一探活模型" })).toHaveTextContent("model-a");
    const failureSummary = screen.getByText("1 个账号获取模型失败，已跳过");
    expect(screen.getByText(/Cloudflare 响应不完整/)).not.toBeVisible();
    await userEvent.click(failureSummary);
    expect(screen.getByText(/Cloudflare 响应不完整/)).toBeVisible();

    const modelE = screen.getByRole("checkbox", { name: /^同步模型 model-e/ });
    expect(modelE).toBeChecked();
    await userEvent.click(modelE);
    expect(modelE).not.toBeChecked();

    await userEvent.click(openaiTab);
    expect(openaiTab).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: /OpenAI 组/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("checkbox", { name: "同步模型 model-b" })).toBeChecked();
    expect(screen.getByRole("button", { name: "同步 2 个账号" })).toBeEnabled();

    await userEvent.click(screen.getByRole("button", { name: "同步 2 个账号" }));
    await waitFor(() =>
      expect(apply).toHaveBeenCalledWith(
        [
          { account_id: "41", models: ["model-a"] },
          { account_id: "42", models: ["model-a", "model-b", "model-e"] },
        ],
        "catalog-fingerprint",
        ["model-a"],
      ),
    );
    const result = await screen.findByTestId("model-sync-apply-result");
    expect(result).toHaveClass("flex", "h-full", "min-h-0");
    expect(screen.getByRole("region", { name: "探活明细" })).toHaveClass("flex-1", "min-h-0");
    expect(screen.getByRole("tabpanel")).toHaveClass("flex-1", "overflow-y-auto");
    expect(screen.getByRole("heading", { name: "同步结果" })).toBeVisible();
    expect(screen.getByText("模型写入")).toBeVisible();
    expect(screen.getByText("探活验证")).toBeVisible();
    expect(screen.getByRole("tab", { name: "失败 1" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("账号 A")).toBeVisible();
    expect(screen.getByText("上游服务暂时不可用（HTTP 503）")).toBeVisible();
    expect(screen.queryByText(/Service temporarily unavailable/)).toBeNull();
    expect(screen.queryByText("账号 B")).toBeNull();

    await userEvent.click(screen.getByRole("tab", { name: "跳过 1" }));
    expect(screen.getByText("账号 B")).toBeVisible();
    expect(screen.getByText("该账号未启用此模型")).toBeVisible();
    expect(screen.queryByText("账号 A")).toBeNull();
  });

  it("同一平台按账号分组切换各自实际发现的模型", async () => {
    const discovery = task([
      { account_id: "41", account_name: "基础账号", status: "succeeded" },
      { account_id: "42", account_name: "高级账号", status: "succeeded" },
    ]);
    const preview: AccountModelSyncPreview = {
      account_count: 2,
      accounts_with_catalog: 2,
      blocked_patterns: [],
      blocked_models: [],
      models: [
        { model: "common-model", account_count: 2 },
        { model: "basic-model", account_count: 1 },
        { model: "advanced-model", account_count: 1 },
      ],
      accounts: [
        {
          account_id: "41",
          account_name: "基础账号",
          platform: "openai",
          models: ["common-model", "basic-model"],
          probe_model: "common-model",
        },
        {
          account_id: "42",
          account_name: "高级账号",
          platform: "openai",
          models: ["common-model", "advanced-model"],
          probe_model: "common-model",
        },
      ],
      fingerprint: "catalog-fingerprint",
    };
    vi.spyOn(api, "discoverAccountModels").mockResolvedValue(discovery);
    vi.spyOn(api, "task").mockResolvedValue(discovery);
    vi.spyOn(api, "previewAccountModels").mockResolvedValue(preview);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <AccountModelSyncDialog
          open
          accountIds={["41", "42"]}
          accountPlatforms={
            new Map([
              ["41", "openai"],
              ["42", "openai"],
            ])
          }
          accountGroups={
            new Map([
              ["41", ["基础组"]],
              ["42", ["高级组"]],
            ])
          }
          onOpenChange={() => undefined}
          onCompleted={() => undefined}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("tablist", { name: "账号平台" })).toBeVisible();
    expect(screen.getByRole("tab", { name: /OpenAI/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tablist", { name: "账号分组" })).toBeVisible();
    expect(screen.getByTestId("unified-probe-models")).toHaveClass("self-start");
    expect(screen.getByTestId("account-sync-models")).toHaveClass(
      "content-start",
      "auto-rows-[3.5rem]",
    );

    await userEvent.click(screen.getByRole("tab", { name: /基础组/ }));
    expect(screen.getByRole("tab", { name: /基础组/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("checkbox", { name: "同步模型 basic-model" })).toBeVisible();
    expect(
      screen.queryByRole("checkbox", { name: "同步模型 advanced-model" }),
    ).not.toBeInTheDocument();
    const basicCommon = screen.getByRole("checkbox", { name: "同步模型 common-model" });
    await userEvent.click(screen.getByRole("group", { name: "模型 common-model" }));
    expect(basicCommon).not.toBeChecked();

    await userEvent.click(screen.getByRole("tab", { name: /高级组/ }));
    expect(screen.getByRole("tab", { name: /高级组/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("checkbox", { name: "同步模型 advanced-model" })).toBeVisible();
    expect(
      screen.queryByRole("checkbox", { name: "同步模型 basic-model" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "同步模型 common-model" })).toBeChecked();

    await userEvent.click(screen.getByRole("tab", { name: /基础组/ }));
    expect(screen.getByRole("checkbox", { name: "同步模型 common-model" })).not.toBeChecked();
    expect(screen.getByTestId("unified-probe-models")).toHaveClass("self-start");
  });

  it("提交时目录发生变化会自动刷新预览并提示重新确认", async () => {
    const discovery = task([{ account_id: "41", account_name: "账号 A", status: "succeeded" }]);
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
          platform: "openai",
          models: ["model-a"],
          probe_model: "model-a",
        },
      ],
      fingerprint: "old-fingerprint",
    };
    const refreshedPreview = { ...preview, fingerprint: "new-fingerprint" };
    const applyTask = {
      ...task([{ account_id: "41", account_name: "账号 A", status: "succeeded" }]),
      id: "model-apply",
      operation: "account-model-apply",
    };
    vi.spyOn(api, "discoverAccountModels").mockResolvedValue(discovery);
    vi.spyOn(api, "task").mockImplementation(async (taskId) =>
      taskId === applyTask.id ? applyTask : discovery,
    );
    const loadPreview = vi
      .spyOn(api, "previewAccountModels")
      .mockResolvedValueOnce(preview)
      .mockResolvedValueOnce(refreshedPreview);
    const apply = vi
      .spyOn(api, "applyAccountModels")
      .mockRejectedValueOnce(
        new ApiError("账号模型目录已变化，请重新获取预览后再应用", "model_catalog_changed", 409),
      )
      .mockResolvedValueOnce(applyTask);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <AccountModelSyncDialog
          open
          accountIds={["41"]}
          accountPlatforms={new Map([["41", "openai"]])}
          accountGroups={new Map([["41", ["基础组"]]])}
          onOpenChange={() => undefined}
          onCompleted={() => undefined}
        />
      </QueryClientProvider>,
    );

    const syncButton = await screen.findByRole("button", { name: "同步 1 个账号" });
    await userEvent.click(syncButton);

    expect(
      await screen.findByText("模型目录刚刚更新，已载入最新结果，请确认后再次同步。"),
    ).toBeVisible();
    expect(loadPreview).toHaveBeenCalledTimes(2);
    expect(screen.queryByText("账号模型目录已变化，请重新获取预览后再应用")).toBeNull();

    await userEvent.click(syncButton);
    await waitFor(() =>
      expect(apply).toHaveBeenLastCalledWith(
        [{ account_id: "41", models: ["model-a"] }],
        "new-fingerprint",
        ["model-a"],
      ),
    );
  });

  it("模型发现任务开始后在固定页脚提供取消入口", async () => {
    const queued = {
      ...task([]),
      status: "queued",
      progress: 0,
      message: "账号模型发现已排队",
    } as Task;
    const running = {
      ...queued,
      status: "running",
      progress: 38,
      message: "正在逐账号获取上游模型目录",
    } as Task;
    vi.spyOn(api, "discoverAccountModels").mockResolvedValue(queued);
    vi.spyOn(api, "task").mockResolvedValue(running);
    const cancel = vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <AccountModelSyncDialog
          open
          accountIds={["41", "42"]}
          accountPlatforms={
            new Map([
              ["41", "anthropic"],
              ["42", "openai"],
            ])
          }
          accountGroups={new Map()}
          onOpenChange={() => undefined}
          onCompleted={() => undefined}
        />
      </QueryClientProvider>,
    );

    const cancelButton = await screen.findByRole("button", { name: "取消任务" });
    const footerCloseButton = screen
      .getAllByRole("button", { name: "关闭" })
      .find((button) => button.textContent === "关闭");
    expect(screen.getByRole("dialog").className).toContain("w-[min(38rem,calc(100vw-2rem))]");
    expect(screen.getByRole("dialog").className).not.toContain("h-[min(52rem,calc(100svh-2rem))]");
    expect(
      screen
        .getAllByRole("button", { name: "关闭" })
        .some((button) => button.hasAttribute("disabled")),
    ).toBe(true);
    expect(cancelButton).toHaveClass("h-8");
    expect(footerCloseButton).toHaveClass("h-8");
    await userEvent.click(cancelButton);
    await waitFor(() => expect(cancel).toHaveBeenCalledWith("model-discovery"));
    expect(screen.getByRole("button", { name: "已请求取消" })).toBeDisabled();
  });
});
