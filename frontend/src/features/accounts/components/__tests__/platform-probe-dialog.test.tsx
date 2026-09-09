import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type AccountStatus, type Task, type TaskSummary } from "@/api";

import {
  PlatformProbeDialog,
  PlatformProbeResultTable,
  partitionPlatformProbeResults,
  platformProbeOptions,
  platformProbeRequest,
  platformProbeResults,
} from "../platform-probe-dialog";

function account(
  id: string,
  platform: string,
  manualPriority: number | null = null,
): AccountStatus {
  return { id, name: `账号 ${id}`, platform, manual_priority: manualPriority } as AccountStatus;
}

function completedTask(): Task {
  return {
    id: "probe-platform-1",
    skill: "sub2api-connectivity-test",
    operation: "active-probe",
    status: "succeeded",
    progress: 100,
    message: "官方探测完成：通过 1，失败 1，跳过 0",
    result: {
      results: [
        {
          account_id: "41",
          account_name: "主账号",
          result: "通过",
          request_model: "gpt-5.6-sol",
          actual_model: "gpt-5.6-sol",
          status_code: 200,
          failure_reason: null,
          duration_ms: 842,
        },
        {
          account_id: "42",
          account_name: "备用账号",
          result: "失败",
          request_model: "gpt-5.6-sol",
          actual_model: "",
          status_code: 404,
          failure_reason: "模型不可用",
          duration_ms: 2180,
        },
      ],
    },
    created_at: "2026-09-05T00:00:00Z",
    updated_at: "2026-09-05T00:00:01Z",
  };
}

describe("平台模型探活", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("按平台精确聚合可探活账号并排除人工优先账号", () => {
    expect(
      platformProbeOptions([
        account("41", "OpenAI"),
        account("42", "openai"),
        account("43", "openai-compatible"),
        account("44", "openai", 1),
      ]),
    ).toEqual([
      { value: "openai", label: "OpenAI", accountCount: 2 },
      { value: "openai-compatible", label: "openai-compatible", accountCount: 1 },
    ]);
  });

  it("模型字段是普通文本输入且显示平台账号范围", () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const view = render(
      <QueryClientProvider client={queryClient}>
        <PlatformProbeDialog
          open
          accounts={[account("41", "openai"), account("42", "openai")]}
          onOpenChange={() => undefined}
        />
      </QueryClientProvider>,
    );

    expect(screen.getByRole("textbox", { name: "输入探活模型" })).not.toHaveAttribute("list");
    expect(view.container.querySelector("datalist")).toBeNull();
    expect(screen.getByText(/2 个账号/)).toBeVisible();
    expect(platformProbeRequest({ platform: " OpenAI ", model: " gpt-5.6-sol " })).toEqual({
      platform: "openai",
      model: "gpt-5.6-sol",
    });
  });

  it("创建任务后立即关闭弹窗并加入系统信息进行中任务", async () => {
    const queuedTask: Task = {
      ...completedTask(),
      id: "probe-background-1",
      status: "queued",
      progress: 0,
      message: "主动探测已排队",
      result: { platform: "openai", model: "gpt-5.4" },
    };
    vi.spyOn(api, "runActiveProbe").mockResolvedValue(queuedTask);
    const taskRead = vi.spyOn(api, "task");
    const onOpenChange = vi.fn();
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData<TaskSummary[]>(
      ["tasks"],
      Array.from({ length: 20 }, (_, index) => ({
        id: `existing-task-${index}`,
        skill: "console",
        operation: "active-probe",
        status: "succeeded",
        progress: 100,
        message: "探活完成",
        system_info: true,
        created_at: `2026-09-04T00:00:${String(index).padStart(2, "0")}Z`,
        updated_at: `2026-09-04T00:00:${String(index).padStart(2, "0")}Z`,
      })),
    );
    render(
      <QueryClientProvider client={queryClient}>
        <PlatformProbeDialog
          open
          accounts={[account("41", "openai")]}
          onOpenChange={onOpenChange}
        />
      </QueryClientProvider>,
    );

    fireEvent.change(screen.getByRole("textbox", { name: "输入探活模型" }), {
      target: { value: "gpt-5.4" },
    });
    fireEvent.click(screen.getByRole("button", { name: "开始探活" }));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(taskRead).not.toHaveBeenCalled();
    expect(queryClient.getQueryData<TaskSummary[]>(["tasks"])?.[0]).toMatchObject({
      id: "probe-background-1",
      status: "queued",
    });
    expect(queryClient.getQueryData<TaskSummary[]>(["tasks"])).toHaveLength(20);
    expect(queryClient.getQueryData<TaskSummary[]>(["tasks"])?.at(-1)?.id).toBe("existing-task-18");
  });

  it("解析结果时保留账号名称、模型、状态码和失败原因", () => {
    expect(platformProbeResults(completedTask())[1]).toEqual({
      accountId: "42",
      accountName: "备用账号",
      result: "失败",
      requestModel: "gpt-5.6-sol",
      actualModel: "",
      statusCode: 404,
      failureReason: "模型不可用",
      durationMS: 2180,
    });
  });

  it("旧任务没有账号名称时按账号 ID 回退到当前账号名称", () => {
    const task = completedTask();
    delete (task.result.results as Array<Record<string, unknown>>)[0].account_name;

    expect(platformProbeResults(task, new Map([["41", "当前账号名称"]]))[0].accountName).toBe(
      "当前账号名称",
    );
  });

  it("将通过结果归为成功，其余结果归为失败并支持分类切换", () => {
    const results = platformProbeResults(completedTask());
    expect(partitionPlatformProbeResults(results)).toMatchObject({
      succeeded: [{ accountName: "主账号" }],
      failed: [{ accountName: "备用账号" }],
    });

    render(<PlatformProbeResultTable results={results} />);

    expect(screen.getByRole("tab", { name: "失败与异常 1" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText("备用账号")).toBeVisible();
    expect(screen.getByText("2.18 秒")).toBeVisible();
    expect(screen.queryByText("主账号")).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: "成功 1" }));
    expect(screen.getByText("主账号")).toBeVisible();
    expect(screen.getByText("ID 41")).toBeVisible();
    expect(screen.queryByText("备用账号")).toBeNull();
  });
});
