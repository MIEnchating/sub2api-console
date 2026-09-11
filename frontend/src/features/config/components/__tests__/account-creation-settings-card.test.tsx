import { Toaster, toast } from "sonner";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { api, ApiError, type AccountCreationSettings, type GroupStatus } from "@/api";
import { AccountCreationSettingsCard } from "../account-creation-settings-card";

const settings: AccountCreationSettings = {
  default: {
    models: ["gpt-5.2"],
    concurrency: 24,
    load_factor: null,
    priority: 3,
    pool_mode: false,
    pool_mode_retry_count: 2,
    pool_mode_retry_status_codes: [429, 503],
  },
  groups: [],
  platform_probe_models: {
    openai: "gpt-5.2",
    anthropic: "claude-sonnet-4-5",
  },
};

const group: GroupStatus = {
  id: "6",
  name: "Claude 专用组",
  account_count: 0,
  scheduling_open: 0,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "balanced",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "empty",
};

const secondGroup: GroupStatus = {
  ...group,
  id: "7",
  name: "OpenAI 高并发组",
};

beforeAll(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  toast.dismiss();
  vi.restoreAllMocks();
});
afterAll(() => vi.unstubAllGlobals());

function renderCard(
  initialScope?: string,
  configuredSettings = settings,
  configuredGroups: GroupStatus[] = [group],
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false } },
  });
  queryClient.setQueryData(["account-creation-settings"], configuredSettings);
  queryClient.setQueryData(["groups"], configuredGroups);
  queryClient.setQueryData(
    ["accounts"],
    [
      { id: "41", name: "账号一" },
      { id: "42", name: "账号二" },
    ],
  );
  return render(
    <QueryClientProvider client={queryClient}>
      <Toaster />
      <AccountCreationSettingsCard
        fallbackConcurrency={10}
        fallbackPriority={1}
        initialScope={initialScope}
      />
    </QueryClientProvider>,
  );
}

describe("账号创建设置卡片", () => {
  it("确认影响范围后启动已有账号池模式同步任务", async () => {
    const user = userEvent.setup();
    const task = {
      id: "pool-sync-1",
      skill: "sub2api-account-pool-mode",
      operation: "account-pool-mode-sync",
      status: "succeeded" as const,
      progress: 100,
      message: "已有账号池模式同步完成",
      result: { succeeded: 2, failed: 0, skipped: 0 },
      created_at: "2026-09-07T00:00:00Z",
      updated_at: "2026-09-07T00:00:01Z",
    };
    const sync = vi.spyOn(api, "syncExistingAccountPoolMode").mockResolvedValue(task);
    vi.spyOn(api, "task").mockResolvedValue(task);
    renderCard();

    await user.click(screen.getByRole("button", { name: "同步池模式到已有账号" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("2 个已有账号");
    expect(screen.getByRole("dialog")).toHaveTextContent(
      "不会修改并发、优先级、账号模型和调度状态",
    );
    await user.click(screen.getByRole("button", { name: "确认同步 2 个账号" }));

    await waitFor(() => expect(sync).toHaveBeenCalledWith(["41", "42"]));
  });

  it("按平台编辑并保存添加账号时的默认探活模型", async () => {
    const user = userEvent.setup();
    const update = vi
      .spyOn(api, "updateAccountCreationSettings")
      .mockImplementation(async (payload) => payload);
    renderCard();

    await user.click(screen.getByRole("tab", { name: "探活模型" }));
    expect(screen.getByLabelText("Composite 默认探活模型")).toBeVisible();
    expect(screen.queryByLabelText("OpenCode 默认探活模型")).not.toBeInTheDocument();
    const openAIModel = screen.getByLabelText("OpenAI 默认探活模型");
    expect(openAIModel).toHaveValue("gpt-5.2");
    await user.clear(openAIModel);
    await user.type(openAIModel, "gpt-5.6-luna");
    await user.click(screen.getByRole("button", { name: "保存默认探活模型" }));

    await waitFor(() => expect(update).toHaveBeenCalledOnce());
    expect(update.mock.calls[0]?.[0].platform_probe_models).toMatchObject({
      openai: "gpt-5.6-luna",
      anthropic: "claude-sonnet-4-5",
    });
  });

  it("开启池模式后展示重试次数与状态码", async () => {
    const user = userEvent.setup();
    renderCard();

    expect(screen.queryByLabelText("重试状态码")).not.toBeInTheDocument();
    screen.getByRole("switch", { name: /池模式/ }).focus();
    await user.keyboard(" ");

    expect(screen.getByLabelText("全局默认 重试次数")).toHaveValue(2);
    expect(screen.getByLabelText("全局默认 重试状态码")).toHaveValue("429, 503");
  });

  it("未配置的分组显示继承状态，开启独立设置后展示独立表单", async () => {
    const user = userEvent.setup();
    renderCard("6");

    expect(screen.getByRole("switch", { name: "Claude 专用组 使用独立设置" })).not.toBeChecked();
    expect(screen.getByText("该分组将使用全局默认配置")).toBeVisible();
    expect(screen.queryByLabelText("Claude 专用组 账号模型")).not.toBeInTheDocument();

    screen.getByRole("switch", { name: "Claude 专用组 使用独立设置" }).focus();
    await user.keyboard(" ");
    expect(screen.getByLabelText("Claude 专用组 账号模型")).toHaveValue("gpt-5.2");
    expect(screen.getByLabelText("Claude 专用组 账号模型")).toBeEnabled();
  });

  it("关闭已有分组独立设置后允许保存以恢复继承", async () => {
    const user = userEvent.setup();
    renderCard("6", {
      ...settings,
      groups: [{ group_id: "6", ...settings.default, concurrency: 8 }],
    });

    const independent = screen.getByRole("switch", {
      name: "Claude 专用组 使用独立设置",
    });
    expect(independent).toBeChecked();
    independent.focus();
    await user.keyboard(" ");

    expect(screen.getByRole("button", { name: "保存继承设置" })).toBeEnabled();
    expect(screen.queryByLabelText("Claude 专用组 账号模型")).not.toBeInTheDocument();
  });

  it("提交无效并发时标记字段并展示可见错误", async () => {
    const user = userEvent.setup();
    renderCard();

    const concurrency = screen.getByLabelText("全局默认 并发上限");
    await user.clear(concurrency);
    await user.click(screen.getByRole("button", { name: "保存全局默认" }));

    expect(concurrency).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("请输入 1 到 10000000 之间的整数")).toBeVisible();
  });

  it("同时列出不同分组及各自已保存的参数", async () => {
    const user = userEvent.setup();
    renderCard(
      undefined,
      {
        ...settings,
        groups: [
          { group_id: "6", ...settings.default, concurrency: 8 },
          {
            group_id: "7",
            ...settings.default,
            models: ["gpt-5.6-sol", "gpt-5.6-terra"],
            concurrency: 64,
            load_factor: "32",
            pool_mode: true,
          },
        ],
      },
      [group, secondGroup],
    );

    await user.click(screen.getByRole("tab", { name: "分组独立配置 2" }));

    const claudeRow = screen.getByTestId("account-group-settings-6");
    const openAIRow = screen.getByTestId("account-group-settings-7");
    expect(within(claudeRow).getByText(/并发 8/)).toBeVisible();
    expect(within(openAIRow).getByText(/2 个模型 · 并发 64 · 负载 32/)).toBeVisible();
    expect(screen.getByText("2 / 2 已配置")).toBeVisible();
  });

  it("保存一个分组时保留其他分组的独立设置", async () => {
    const user = userEvent.setup();
    const configuredSettings: AccountCreationSettings = {
      ...settings,
      groups: [
        { group_id: "6", ...settings.default, concurrency: 8 },
        { group_id: "7", ...settings.default, concurrency: 64 },
      ],
    };
    const update = vi
      .spyOn(api, "updateAccountCreationSettings")
      .mockImplementation(async (payload) => payload);
    renderCard("6", configuredSettings, [group, secondGroup]);

    const concurrency = screen.getByLabelText("Claude 专用组 并发上限");
    await user.clear(concurrency);
    await user.type(concurrency, "12");
    await user.click(screen.getByRole("button", { name: "保存 Claude 专用组 设置" }));

    await waitFor(() => expect(update).toHaveBeenCalledOnce());
    const payload = update.mock.calls[0]?.[0];
    expect(payload?.groups.find((item) => item.group_id === "6")?.concurrency).toBe(12);
    expect(payload?.groups.find((item) => item.group_id === "7")?.concurrency).toBe(64);
  });

  it("后端缺少账号设置接口时仅悬浮提示请求失败", async () => {
    vi.spyOn(api, "accountCreationSettings").mockRejectedValue(
      new ApiError("请求失败（404）", "http_error", 404),
    );
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY } },
    });
    queryClient.setQueryData(["groups"], [group]);
    render(
      <QueryClientProvider client={queryClient}>
        <Toaster />
        <AccountCreationSettingsCard fallbackConcurrency={10} fallbackPriority={1} />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("请求失败（404）")).toBeVisible();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
