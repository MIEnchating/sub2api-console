import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type AccountDetail, type Task } from "@/api";
import { accountDetailDialogLayout } from "../account-detail-dialog";
import {
  AccountSettingsPanel,
  accountTestModelOptions,
  waitForAccountSettingTasks,
} from "../account-settings-panel";

const detail: AccountDetail = {
  id: "41",
  name: "channel-41",
  groups: ["codex"],
  upstream_id: "up_test",
  upstream_host: "api.example.test",
  upstream_type: "apikey",
  base_url: "https://account-api.example.test/v1",
  upstream_base_url: "https://api.example.test",
  schedulable: true,
  priority: 753,
  load_factor: "1",
  concurrency: 3000,
  multiplier: "0.08",
  balance: null,
  paused: false,
  paused_reason: null,
  routing_state: "healthy",
  health_status: "healthy",
  health: "healthy",
  desired_health: "healthy",
  apply_pending: false,
  apply_error: null,
  decision_state: "healthy",
  decision_reason: null,
  failure_streak: 0,
  recovery_pass_streak: 0,
  target_priority: 120,
  target_load_factor: "1",
  target_schedulable: true,
  target_concurrency: 3000,
  health_score: 100,
  short_score: 100,
  long_score: 100,
  sample_count: 1,
  recent_results: [],
  ttfb_p50_ms: 100,
  ttfb_p95_ms: 120,
  weight: 100,
  metadata: {},
  group_rates: { codex: "0.08" },
  group_ids: { codex: "7" },
  bindings: [],
  test_models: ["gpt-5.1-codex", "claude-sonnet-4"],
};

function renderPanel() {
  const queryClient = new QueryClient();
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <AccountSettingsPanel
        accountId="41"
        query={{ data: detail, isLoading: false, isError: false, error: null }}
        onCancel={() => undefined}
        onSaved={() => undefined}
      />
    </QueryClientProvider>,
  );
}

describe("账号设置面板", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("获取模型后去重排序并保留当前单个探测模型供选择", () => {
    expect(
      accountTestModelOptions(["gpt-5.2", "gpt-5.1-codex", "gpt-5.2", ""], "custom-probe-model"),
    ).toEqual(["custom-probe-model", "gpt-5.1-codex", "gpt-5.2"]);
  });

  it("matches channel settings and omits group editing and multiplier breaker fields", () => {
    const markup = renderPanel();

    for (const label of [
      "优先级",
      "负载因子",
      "并发上限",
      "账号成本",
      "暂停调度",
      "排除该账号",
      "探测模型",
    ]) {
      expect(markup).toContain(label);
    }
    expect(markup).not.toContain("账号分组");
    expect(markup).not.toContain("倍率超阈值");
    expect(markup).not.toContain("账号名称");
    expect(markup).not.toContain("备注");
    expect(markup).toContain('aria-readonly="true"');
    expect(markup).toContain('aria-label="优先级说明"');
    expect(markup).toContain('aria-label="账号成本说明"');
    expect(markup).toContain('aria-label="暂停调度说明"');
    expect(markup).toContain('aria-label="排除该账号说明"');
    expect(markup).not.toContain("请使用“同步倍率”更新");
    expect(markup).not.toContain("停止接收流量，继续监控计分，不自动恢复。");
    expect(markup).not.toContain("上游 Host");
    expect(markup).not.toContain("账号 Base URL");
    expect(markup).toContain('aria-label="探测模型"');
    expect(markup).toContain("只使用一个模型进行实际验证");
    expect(markup).not.toContain("多个模型逐一探测");
  });

  it("makes the complete pause and exclude rows operable buttons", () => {
    const markup = renderPanel();

    expect(markup).toContain('aria-label="暂停调度"');
    expect(markup).toContain('aria-label="排除该账号"');
    expect(markup).toContain('for="account-settings-41-paused"');
    expect(markup).toContain('for="account-settings-41-excluded"');
  });

  it("uses a compact two-column form without nested highlight cards", () => {
    const markup = renderPanel();

    expect(accountDetailDialogLayout.content).not.toContain("sm:max-w-");
    expect(markup).toContain('data-testid="account-routing-grid"');
    expect(markup).toContain("sm:grid-cols-2");
    expect(markup).toContain('data-testid="account-control-group"');
    expect(markup).toContain("rounded-xl border bg-muted/10");
    expect(markup).not.toContain("border-primary/25");
    expect(markup).not.toContain("bg-primary/5");
  });

  it("保存任务开始后提供取消入口并取消精确任务", async () => {
    const queued = {
      id: "settings-task-41",
      skill: "sub2api-account-management",
      operation: "account-settings",
      status: "queued",
      progress: 0,
      message: "账号设置已排队",
      result: {},
      created_at: "2026-09-05T00:00:00Z",
      updated_at: "2026-09-05T00:00:00Z",
    } as Task;
    const save = vi.spyOn(api, "saveAccountSettings").mockResolvedValue(queued);
    vi.spyOn(api, "task").mockImplementation(() => new Promise<Task>(() => undefined));
    const cancel = vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <AccountSettingsPanel
          accountId="41"
          query={{ data: detail, isLoading: false, isError: false, error: null }}
          onCancel={() => undefined}
          onSaved={() => undefined}
        />
      </QueryClientProvider>,
    );

    await userEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith(
        "41",
        expect.objectContaining({ test_models: ["gpt-5.1-codex"] }),
      ),
    );
    const cancelButton = await screen.findByRole("button", { name: "取消任务" });
    await userEvent.click(cancelButton);

    await waitFor(() => expect(cancel).toHaveBeenCalledWith("settings-task-41"));
  });
});

describe("账号设置保存流程", () => {
  it("等待排队任务完成后才返回", async () => {
    const queued = { id: "task-1", status: "queued" } as Task;
    const succeeded = { ...queued, status: "succeeded", message: "完成" } as Task;
    const load = vi.fn(async () => succeeded);
    const wait = vi.fn(async () => undefined);

    await expect(waitForAccountSettingTasks([queued], load, wait)).resolves.toEqual([succeeded]);
    expect(load).toHaveBeenCalledWith("task-1");
  });

  it("任务失败时保留后端失败原因", async () => {
    const queued = { id: "task-1", status: "queued" } as Task;
    const failed = { ...queued, status: "failed", message: "远端读回失败" } as Task;

    await expect(
      waitForAccountSettingTasks(
        [queued],
        async () => failed,
        async () => undefined,
      ),
    ).rejects.toThrow("远端读回失败");
  });
});

it("账号设置读取中显示轻量提示并禁用保存，取消仍可操作", async () => {
  const client = new QueryClient();
  const cancel = vi.fn();
  const view = render(
    <QueryClientProvider client={client}>
      <AccountSettingsPanel
        accountId="41"
        query={{ isLoading: true, isError: false, error: null }}
        onCancel={cancel}
        onSaved={vi.fn()}
      />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("status", { name: "正在读取账号设置" })).toHaveTextContent(
    "正在读取账号设置",
  );
  expect(view.container.querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  await userEvent.setup().click(screen.getByRole("button", { name: "取消" }));
  expect(cancel).toHaveBeenCalledOnce();
  view.unmount();
  client.clear();
});
