import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
  type UseQueryResult,
} from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, type AnimationRequest, type Task } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskIsTerminal, taskPollInterval } from "@/lib/task-state";
import { registerBackgroundTask } from "../lib/register-background-task";
import { collectAnimationTasks } from "../lib/animation-task-results";
import { taskEventIsOlder } from "../lib/task-event-schema";
import { createAnimationTaskReader } from "../lib/animation-task-reader";

type AnimationQueries = {
  tasks: (Task | undefined)[];
  isError: boolean;
  isLoading: boolean;
  isFetching: boolean;
  failedQueries: UseQueryResult<Task>["refetch"][];
};

export function useAnimationTasks(active = true) {
  const client = useQueryClient();
  const [readTask] = useState(createAnimationTaskReader);
  const [createdIDs, setCreatedIDs] = useState<string[]>([]);
  const displayedIDs = useRef(new Set<string>());
  const confidenceVersions = useRef(new Map<string, string>());
  const submittingRef = useRef(new Set<string>());
  const [submitting, setSubmitting] = useState(new Set<string>());
  const precheckSubmittingRef = useRef(new Set<string>());
  const [precheckSubmitting, setPrecheckSubmitting] = useState(new Set<string>());
  const history = useQuery({
    queryKey: ["model-animation", "history"],
    queryFn: ({ signal }) => api.animationHistory(signal),
    refetchOnMount: (query) =>
      query.state.data === undefined || query.state.isInvalidated ? "always" : false,
    refetchInterval: active ? 15_000 : false,
  });
  const ids = useMemo(
    () => [...new Set([...createdIDs, ...(history.data ?? []).map((task) => task.id)])],
    [createdIDs, history.data],
  );
  const historyByID = useMemo(
    () => new Map((history.data ?? []).map((task) => [task.id, task])),
    [history.data],
  );
  useEffect(() => {
    if (!history.isSuccess) return;
    setCreatedIDs((current) => {
      const pending = current.filter((id) => !historyByID.has(id));
      return pending.length === current.length ? current : pending;
    });
  }, [history.isSuccess, historyByID]);
  useEffect(() => {
    if (!active) return;
    for (const summary of historyByID.values()) {
      const queryKey = ["model-animation", "task", summary.id];
      const cached = client.getQueryData<Task>(queryKey);
      // History carries versions, not full results. Keep cached content while
      // refreshing changes made by another page or session, including terminals.
      if (cached && taskEventIsOlder(summary, cached)) {
        void client.invalidateQueries({ queryKey, exact: true }, { cancelRefetch: false });
      }
    }
  }, [active, client, historyByID, history.dataUpdatedAt]);
  const combineQueries = useCallback(
    (queries: UseQueryResult<Task>[]): AnimationQueries => ({
      tasks: ids.map((id, index) => queries[index]?.data ?? historyByID.get(id)),
      isError: queries.some((query) => query.isError),
      isLoading: queries.some((query) => query.isLoading),
      isFetching: queries.some((query) => query.isFetching),
      failedQueries: queries.filter((query) => query.isError).map((query) => query.refetch),
    }),
    [ids, historyByID],
  );
  const queries = useQueries({
    combine: combineQueries,
    queries: ids.map((id) => ({
      queryKey: ["model-animation", "task", id],
      queryFn: ({ signal }) => {
        const cached = client.getQueryData<Task>(["model-animation", "task", id]);
        const summary = historyByID.get(id);
        const latest = cached && !taskEventIsOlder(summary, cached) ? cached : summary;
        return readTask(id, signal, !taskIsTerminal(latest));
      },
      enabled: active,
      staleTime: Infinity,
      refetchInterval: (query: { state: { status: string; data?: Task } }) =>
        active ? taskPollInterval(query, 1000) : false,
    })),
  });
  useEffect(() => {
    let changed = false;
    for (const task of queries.tasks) {
      if (!task || !taskIsTerminal(task)) continue;
      if (confidenceVersions.current.get(task.id) === task.updated_at) continue;
      confidenceVersions.current.set(task.id, task.updated_at);
      changed = true;
    }
    if (changed) void client.invalidateQueries({ queryKey: ["model-check-account-statuses"] });
  }, [client, queries.tasks]);
  useEffect(() => {
    for (const id of ids) displayedIDs.current.add(id);
    if (!history.isSuccess || !history.isFetchedAfterMount || history.isFetching) return;
    const retainedIDs = new Set(ids);
    for (const id of displayedIDs.current) {
      if (retainedIDs.has(id)) continue;
      const queryKey = ["model-animation", "task", id];
      const query = client.getQueryCache().find({ queryKey, exact: true });
      if (query?.getObserversCount()) continue;
      if (query) client.removeQueries({ queryKey, exact: true });
      displayedIDs.current.delete(id);
    }
  }, [
    client,
    history.isSuccess,
    history.isFetchedAfterMount,
    history.isFetching,
    history.dataUpdatedAt,
    ids,
  ]);
  // Query's combine keeps unchanged task data stable even when fetch flags change.
  const state = useMemo(
    () => collectAnimationTasks(queries.tasks, submitting, precheckSubmitting),
    [queries.tasks, submitting, precheckSubmitting],
  );
  const run = useMutation({
    mutationFn: api.runAnimation,
    gcTime: 0,
    onSuccess: (created) => {
      registerBackgroundTask(client, created);
      client.setQueryData(["model-animation", "task", created.id], created);
      setCreatedIDs((current) => [...new Set([...current, created.id])]);
      void client.invalidateQueries({ queryKey: ["model-animation", "history"] });
    },
    onError: (error, request) =>
      notifyOperationError(
        error,
        request.mode === "precheck" ? "前置检测启动失败" : "动画检测启动失败",
      ),
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
  const resetMutation = run.reset;
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
      if (request.mode === "precheck") {
        for (const target of request.targets) precheckSubmittingRef.current.add(target.account_id);
        setPrecheckSubmitting(new Set(precheckSubmittingRef.current));
      }
      setSubmitting(new Set(submittingRef.current));
      try {
        await mutateAsync(request);
        return true;
      } catch {
        return false;
      } finally {
        resetMutation();
        for (const target of request.targets) submittingRef.current.delete(target.account_id);
        for (const target of request.targets)
          precheckSubmittingRef.current.delete(target.account_id);
        setPrecheckSubmitting(new Set(precheckSubmittingRef.current));
        setSubmitting(new Set(submittingRef.current));
      }
    },
    [client, ids, mutateAsync, resetMutation],
  );
  const cancelActive = useCallback(async (): Promise<void> => {
    const taskIDs = [...state.activeTaskIDs];
    await Promise.all(taskIDs.map((taskID) => cancel.mutateAsync(taskID).catch(() => undefined)));
  }, [cancel, state.activeTaskIDs]);
  return {
    ...state,
    start,
    historyError: history.isError || queries.isError,
    historyLoading: history.isPending || queries.isLoading,
    retryingHistory: history.isFetching || queries.isFetching,
    retryHistory: (): void => {
      if (history.isError) void history.refetch();
      for (const refetch of queries.failedQueries) void refetch();
    },
    cancelActive,
    cancelling: cancel.isPending,
  };
}
