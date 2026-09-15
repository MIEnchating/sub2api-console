import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Task } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { useOnboardingModelOptions } from "../use-onboarding-model-options";

const clients: QueryClient[] = [];

function task(id: string, status: Task["status"], result: Task["result"] = {}): Task {
  return {
    id,
    status,
    result,
    operation: "onboarding-model-options",
    skill: "onboarding",
    progress: status === "running" ? 30 : 100,
    message: status === "failed" ? "读取模型失败，请重试" : "模型列表读取完成",
    created_at: "2026-09-15T00:00:00Z",
    updated_at: "2026-09-15T00:00:01Z",
  };
}

function renderOptions(
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  clients.push(client);
  return renderHook(
    (props: { host: string; groupId: string }) =>
      useOnboardingModelOptions(props.host, props.groupId),
    {
      initialProps: { host: "models.example.test", groupId: "group-a" },
      wrapper: (props: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
      ),
    },
  );
}

beforeEach(() => {
  vi.spyOn(api, "startOnboardingProbeTask").mockResolvedValue(task("models-1", "queued"));
  vi.spyOn(api, "task").mockResolvedValue(task("models-1", "succeeded", { models: ["gpt-5"] }));
  vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
  vi.spyOn(toast, "error").mockReturnValue("model-error");
});

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

describe("开户模型选项", () => {
  it("进入页面时保持未读取状态，不自动创建或查询任务", () => {
    const view = renderOptions();

    expect(view.result.current).toMatchObject({
      models: [],
      loaded: false,
      pending: false,
      failed: false,
    });
    expect(api.startOnboardingProbeTask).not.toHaveBeenCalled();
    expect(api.task).not.toHaveBeenCalled();
  });

  it("手动读取成功后缓存去重且非空的模型，不返回任务的其他字段", async () => {
    vi.mocked(api.task).mockResolvedValue(
      task("models-1", "succeeded", {
        models: [" gpt-5 ", "", "gpt-5", "claude-sonnet", "  "],
        key: "private-key-must-not-leak",
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const view = renderOptions(client);

    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.loaded).toBe(true));

    expect(api.startOnboardingProbeTask).toHaveBeenCalledWith(
      "model-options",
      "models.example.test",
      "group-a",
    );
    expect(api.task).toHaveBeenCalledWith("models-1");
    expect(view.result.current).toMatchObject({
      models: ["gpt-5", "claude-sonnet"],
      pending: false,
      failed: false,
    });
    view.unmount();
    const reopened = renderOptions(client);
    expect(reopened.result.current.models).toEqual(["gpt-5", "claude-sonnet"]);
    expect(reopened.result.current.loaded).toBe(true);
    expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1);
  });

  it("任务创建尚未完成时连续点击只提交一次，卸载不会取消或清理后台任务", async () => {
    let resolveStart!: (value: Task) => void;
    vi.mocked(api.startOnboardingProbeTask).mockReturnValue(
      new Promise((resolve) => {
        resolveStart = resolve;
      }),
    );
    const view = renderOptions();

    act(() => {
      view.result.current.load();
      view.result.current.load();
    });
    await waitFor(() => expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1));
    expect(view.result.current.pending).toBe(true);
    view.unmount();
    await act(async () => resolveStart(task("models-1", "queued")));

    expect(api.cancelTask).not.toHaveBeenCalled();
    expect(api.task).not.toHaveBeenCalled();
  });

  it("刷新失败时保留旧模型，提示一次且允许明确重试", async () => {
    const view = renderOptions();
    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.loaded).toBe(true));
    vi.mocked(api.startOnboardingProbeTask).mockResolvedValue(task("models-2", "queued"));
    vi.mocked(api.task).mockResolvedValue(task("models-2", "failed"));

    act(() => view.result.current.load());
    expect(view.result.current.models).toEqual(["gpt-5"]);
    await waitFor(() => expect(view.result.current.failed).toBe(true));
    view.rerender({ host: "models.example.test", groupId: "group-a" });

    expect(view.result.current).toMatchObject({ models: ["gpt-5"], loaded: true, pending: false });
    expect(toast.error).toHaveBeenCalledTimes(1);
    vi.mocked(api.startOnboardingProbeTask).mockResolvedValue(task("models-3", "queued"));
    vi.mocked(api.task).mockResolvedValue(task("models-3", "succeeded", { models: ["new-model"] }));
    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.models).toEqual(["new-model"]));
    expect(view.result.current.failed).toBe(false);
  });

  it("任务运行中继续轮询直到成功，重复读取不重新创建任务", async () => {
    vi.mocked(api.task)
      .mockResolvedValueOnce(task("models-1", "running"))
      .mockResolvedValue(task("models-1", "succeeded", { models: ["finished-model"] }));
    const view = renderOptions();
    act(() => view.result.current.load());
    await waitFor(() => expect(api.task).toHaveBeenCalledWith("models-1"));
    expect(view.result.current.pending).toBe(true);

    act(() => {
      view.result.current.load();
      view.result.current.load();
    });
    await waitFor(() => expect(view.result.current.models).toEqual(["finished-model"]));

    expect(view.result.current.pending).toBe(false);
    expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1);
    expect(api.task).toHaveBeenCalledTimes(2);
  });

  it.each([[], ["", "  "], [123], undefined])(
    "终态 models 为 %j 时标记失败并只显示可操作提示",
    async (models) => {
      vi.mocked(api.task).mockResolvedValue(
        task("models-1", "succeeded", { models, credential: "private-key-must-not-leak" }),
      );
      const view = renderOptions();
      act(() => view.result.current.load());
      await waitFor(() => expect(view.result.current.failed).toBe(true));

      expect(view.result.current).toMatchObject({ models: [], loaded: false, pending: false });
      expect(toast.error).toHaveBeenCalledTimes(1);
      expect(JSON.stringify(vi.mocked(toast.error).mock.calls)).not.toContain(
        "private-key-must-not-leak",
      );
    },
  );

  it("创建失败时不自动重放，失败状态供页面提供重试入口", async () => {
    vi.mocked(api.startOnboardingProbeTask).mockRejectedValue(
      new Error("模型任务启动失败，请重试"),
    );
    const view = renderOptions();
    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.failed).toBe(true));

    expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1);
    expect(api.task).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalledTimes(1);
  });

  it("查询传输错误只由全局提示，手动重试继续查询原任务", async () => {
    vi.mocked(api.task).mockRejectedValueOnce(new Error("任务读取失败，请重试"));
    const view = renderOptions(createConsoleQueryClient());
    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.failed).toBe(true));
    expect(toast.error).toHaveBeenCalledTimes(1);

    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.loaded).toBe(true));
    expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1);
    expect(vi.mocked(api.task).mock.calls).toEqual([["models-1"], ["models-1"]]);
  });

  it("切换上游和分组后旧任务结果不能覆盖新目标的模型", async () => {
    let resolveOld!: (value: Task) => void;
    vi.mocked(api.task).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
    );
    const view = renderOptions();
    act(() => view.result.current.load());
    await waitFor(() => expect(api.task).toHaveBeenCalledWith("models-1"));
    view.rerender({ host: "other.example.test", groupId: "group-b" });
    expect(view.result.current).toMatchObject({ models: [], loaded: false, pending: false });
    vi.mocked(api.startOnboardingProbeTask).mockResolvedValue(task("models-2", "queued"));
    vi.mocked(api.task).mockResolvedValue(
      task("models-2", "succeeded", { models: ["target-model"] }),
    );
    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.models).toEqual(["target-model"]));

    await act(async () => resolveOld(task("models-1", "succeeded", { models: ["old-model"] })));

    expect(view.result.current.models).toEqual(["target-model"]);
    expect(api.startOnboardingProbeTask).toHaveBeenLastCalledWith(
      "model-options",
      "other.example.test",
      "group-b",
    );
  });

  it("同上游同分组的两个账号同时读取时共享进行状态且仅创建一个任务", async () => {
    let resolveStart!: (value: Task) => void;
    vi.mocked(api.startOnboardingProbeTask).mockReturnValue(
      new Promise((resolve) => {
        resolveStart = resolve;
      }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const first = renderOptions(client);
    const second = renderOptions(client);

    act(() => {
      first.result.current.load();
      second.result.current.load();
    });
    await waitFor(() =>
      expect(first.result.current.pending && second.result.current.pending).toBe(true),
    );
    expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1);
    await act(async () => resolveStart(task("models-1", "queued")));
    await waitFor(() =>
      expect(first.result.current.loaded && second.result.current.loaded).toBe(true),
    );

    expect(first.result.current.models).toEqual(["gpt-5"]);
    expect(second.result.current.models).toEqual(["gpt-5"]);
    expect(api.task).toHaveBeenCalledTimes(1);
  });

  it("共享任务失败时多个账号均可重试且业务错误只提示一次", async () => {
    vi.mocked(api.task).mockResolvedValue(task("models-1", "failed"));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const first = renderOptions(client);
    const second = renderOptions(client);
    act(() => first.result.current.load());
    await waitFor(() =>
      expect(first.result.current.failed && second.result.current.failed).toBe(true),
    );

    expect(first.result.current.pending).toBe(false);
    expect(second.result.current.pending).toBe(false);
    expect(toast.error).toHaveBeenCalledTimes(1);
  });

  it("任务创建期间切换目标时旧创建结果只属于原目标", async () => {
    let resolveStart!: (value: Task) => void;
    vi.mocked(api.startOnboardingProbeTask).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveStart = resolve;
      }),
    );
    const view = renderOptions();
    act(() => view.result.current.load());
    await waitFor(() => expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1));
    view.rerender({ host: "other.example.test", groupId: "group-b" });
    vi.mocked(api.startOnboardingProbeTask).mockResolvedValue(task("models-2", "queued"));
    vi.mocked(api.task).mockResolvedValue(
      task("models-2", "succeeded", { models: ["target-model"] }),
    );
    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.loaded).toBe(true));

    await act(async () => resolveStart(task("models-1", "queued")));

    expect(view.result.current.models).toEqual(["target-model"]);
    expect(api.task).not.toHaveBeenCalledWith("models-1");
  });

  it.each([
    { host: "other.example.test", groupId: "group-a" },
    { host: "models.example.test", groupId: "group-b" },
  ])("切换为 $host / $groupId 时不复用其他目标的成功列表", async (target) => {
    const view = renderOptions();
    act(() => view.result.current.load());
    await waitFor(() => expect(view.result.current.loaded).toBe(true));

    view.rerender(target);

    expect(view.result.current).toMatchObject({
      models: [],
      loaded: false,
      pending: false,
      failed: false,
    });
    expect(api.startOnboardingProbeTask).toHaveBeenCalledTimes(1);
    view.rerender({ host: "models.example.test", groupId: "group-a" });
    expect(view.result.current.models).toEqual(["gpt-5"]);
  });
});
