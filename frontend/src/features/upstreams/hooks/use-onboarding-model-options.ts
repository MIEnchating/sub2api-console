import { useCallback, useEffect, useId, useRef } from "react";
import { skipToken, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";

import { api } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskIsTerminal, taskPollInterval } from "@/lib/task-state";

const modelOptionsSchema = z.object({ models: z.array(z.string()) });
const emptyModels: string[] = [];
const modelOptionsMessages = {
  failed: "模型列表读取失败，请重试",
  invalid: "模型列表格式无效，请重试或检查上游模型接口",
  empty: "上游未返回可用模型，请检查分组权限后重试",
  cancelled: "模型列表读取已取消，请重试",
} as const;

type Attempt = {
  id: string;
  host: string;
  groupId: string;
  taskId: string | null;
  status: "starting" | "running" | "succeeded" | "failed";
};

type ModelOptions = {
  models: string[];
  pending: boolean;
  failed: boolean;
  loaded: boolean;
  load: () => void;
};

function attemptKey(host: string, groupId: string): readonly string[] {
  return ["onboarding-model-options-request", host, groupId];
}

export function useOnboardingModelOptions(host: string, groupId: string): ModelOptions {
  const client = useQueryClient();
  const instanceId = useId();
  const sequence = useRef(0);
  const attempt = useQuery<Attempt>({ queryKey: attemptKey(host, groupId), queryFn: skipToken });
  const current = attempt.data;
  const models = useQuery<string[]>({
    queryKey: ["onboarding-model-options", host, groupId],
    queryFn: skipToken,
    staleTime: Infinity,
  });
  const taskId = current?.taskId;
  const task = useQuery({
    queryKey: ["onboarding-model-options-task", taskId ?? null],
    queryFn: taskId ? () => api.task(taskId) : skipToken,
    enabled: current?.status === "running",
    retry: false,
    refetchInterval: taskPollInterval,
  });
  const start = useMutation({
    mutationFn: (request: Attempt) =>
      api.startOnboardingProbeTask("model-options", request.host, request.groupId),
    retry: false,
    onSuccess: (created, request) => {
      const key = attemptKey(request.host, request.groupId);
      if (client.getQueryData<Attempt>(key)?.id !== request.id) return;
      client.setQueryData(key, { ...request, taskId: created.id, status: "running" });
    },
    onError: (error, request) => {
      const key = attemptKey(request.host, request.groupId);
      if (client.getQueryData<Attempt>(key)?.id !== request.id) return;
      client.setQueryData(key, { ...request, status: "failed" });
      notifyOperationError(error, modelOptionsMessages.failed);
    },
  });

  useEffect(() => {
    const completed = task.data;
    if (!current || completed?.id !== current.taskId || !taskIsTerminal(completed)) return;
    const key = attemptKey(current.host, current.groupId);
    const latest = client.getQueryData<Attempt>(key);
    if (latest?.id !== current.id || latest.status !== "running") return;

    let failure: string | null = null;
    let options: string[] = [];
    if (completed.status !== "succeeded") {
      failure = completed.message.trim() || modelOptionsMessages.failed;
      if (completed.status === "cancelled") failure = modelOptionsMessages.cancelled;
    } else {
      const parsed = modelOptionsSchema.safeParse(completed.result);
      if (!parsed.success) failure = modelOptionsMessages.invalid;
      else {
        options = [...new Set(parsed.data.models.map((model) => model.trim()).filter(Boolean))];
        if (options.length === 0) failure = modelOptionsMessages.empty;
      }
    }
    client.setQueryData(key, { ...current, status: failure ? "failed" : "succeeded" });
    if (failure) notifyOperationError(new Error(failure), modelOptionsMessages.failed);
    else client.setQueryData(["onboarding-model-options", current.host, current.groupId], options);
  }, [client, current, task.data]);

  const load = useCallback((): void => {
    const key = attemptKey(host, groupId);
    const latest = client.getQueryData<Attempt>(key);
    if (latest?.status === "starting" || latest?.status === "running") {
      if (latest.taskId && task.isError) void task.refetch({ cancelRefetch: false });
      return;
    }
    const request: Attempt = {
      id: `${instanceId}:${++sequence.current}`,
      host,
      groupId,
      taskId: null,
      status: "starting",
    };
    client.setQueryData(key, request);
    start.mutate(request);
  }, [client, groupId, host, instanceId, start.mutate, task.isError, task.refetch]);

  return {
    models: models.data ?? emptyModels,
    loaded: models.data !== undefined,
    failed: current?.status === "failed" || (current?.status === "running" && task.isError),
    pending:
      current?.status === "starting" ||
      (current?.status === "running" && (!task.isError || task.isFetching)),
    load,
  };
}
