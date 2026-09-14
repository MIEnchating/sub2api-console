import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo, useRef, useState } from "react";
import { api, type AnimationRequest, type Task } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskPollInterval } from "@/lib/task-state";
import { collectAnimationTasks } from "../lib/animation-task-results";

export function useAnimationTasks(active = true) {
  const client = useQueryClient();
  const [createdIDs, setCreatedIDs] = useState<string[]>([]);
  const submittingRef = useRef(new Set<string>());
  const [submitting, setSubmitting] = useState(new Set<string>());
  const history = useQuery({
    queryKey: ["model-animation", "history"],
    queryFn: api.animationHistory,
    refetchOnMount: (query) => (query.state.data === undefined ? "always" : false),
    refetchInterval: active ? 15_000 : false,
  });
  const ids = useMemo(
    () => [...new Set([...createdIDs, ...(history.data ?? []).map((task) => task.id)])],
    [createdIDs, history.data],
  );
  const queries = useQueries({
    queries: ids.map((id) => ({
      queryKey: ["model-animation", "task", id],
      queryFn: () => api.task(id),
      enabled: active,
      staleTime: Infinity,
      refetchInterval: (query: { state: { status: string; data?: Task } }) =>
        active ? taskPollInterval(query, 1000) : false,
    })),
  });
  const tasks = ids.map((id, index) => queryDataByID(id, queries[index]?.data, history.data));
  const state = collectAnimationTasks(tasks, submitting);
  const run = useMutation({
    mutationFn: api.runAnimation,
    onSuccess: (created) => {
      client.setQueryData(["model-animation", "task", created.id], created);
      setCreatedIDs((current) => [...new Set([...current, created.id])]);
      void client.invalidateQueries({ queryKey: ["model-animation", "history"] });
      void client.invalidateQueries({ queryKey: ["tasks"] });
    },
    onError: (error) => notifyOperationError(error, "动画检测启动失败"),
  });
  const cancel = useMutation({
    mutationFn: api.cancelTask,
    onSuccess: (_result, taskID) => {
      void client.invalidateQueries({ queryKey: ["model-animation", "task", taskID] });
      void client.invalidateQueries({ queryKey: ["model-animation", "history"] });
      void client.invalidateQueries({ queryKey: ["tasks"] });
    },
    onError: (error) => notifyOperationError(error, "动画检测取消失败"),
  });
  const mutateAsync = run.mutateAsync;
  const start = useCallback(
    async (request: AnimationRequest): Promise<boolean> => {
      // Guard synchronously too: a second click can precede React's next render.
      const currentTasks = ids.map((id) =>
        client.getQueryData<Task>(["model-animation", "task", id]),
      );
      const busy = collectAnimationTasks(currentTasks, submittingRef.current).busyIDs;
      if (request.targets.some((target) => busy.has(target.account_id))) {
        notifyOperationError(
          new Error("所选账号正在执行动画检测，请选择其他账号"),
          "动画检测启动失败",
        );
        return false;
      }
      for (const target of request.targets) submittingRef.current.add(target.account_id);
      setSubmitting(new Set(submittingRef.current));
      try {
        await mutateAsync(request);
        return true;
      } catch {
        return false;
      } finally {
        for (const target of request.targets) submittingRef.current.delete(target.account_id);
        setSubmitting(new Set(submittingRef.current));
      }
    },
    [client, ids, mutateAsync],
  );
  const cancelActive = useCallback(async (): Promise<void> => {
    const taskIDs = [...state.activeTaskIDs];
    await Promise.all(taskIDs.map((taskID) => cancel.mutateAsync(taskID).catch(() => undefined)));
  }, [cancel, state.activeTaskIDs]);
  return {
    ...state,
    start,
    historyError: history.isError || queries.some((query) => query.isError),
    retryingHistory: history.isFetching || queries.some((query) => query.isFetching),
    retryHistory: (): void => {
      if (history.isError) void history.refetch();
      for (const query of queries) if (query.isError) void query.refetch();
    },
    cancelActive,
    cancelling: cancel.isPending,
  };
}

function queryDataByID(
  id: string,
  data: Task | undefined,
  history: Task[] | undefined,
): Task | undefined {
  return data ?? history?.find((task) => task.id === id);
}
