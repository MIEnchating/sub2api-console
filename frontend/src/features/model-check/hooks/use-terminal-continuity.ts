import { useMemo, useState } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Task, type TerminalContinuityRequest } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskPollInterval } from "@/lib/task-state";
import {
  isTerminalContinuityActive,
  terminalContinuityBusyIDs,
  terminalContinuityResults,
} from "../lib/terminal-continuity";

const historyKey = ["terminal-continuity", "history"];

export function useTerminalContinuity() {
  const client = useQueryClient();
  const [createdIDs, setCreatedIDs] = useState<string[]>([]);
  const history = useQuery({
    queryKey: historyKey,
    queryFn: api.terminalContinuityHistory,
    refetchInterval: 15_000,
  });
  const ids = useMemo(
    () => [...new Set([...(history.data ?? []).map((task) => task.id), ...createdIDs])],
    [createdIDs, history.data],
  );
  const queries = useQueries({
    queries: ids.map((id) => ({
      queryKey: ["terminal-continuity", "task", id],
      queryFn: () => api.task(id),
      initialData: history.data?.find((task) => task.id === id),
      refetchInterval: taskPollInterval,
    })),
  });
  const tasks = ids.flatMap((id, index) => {
    const task = queries[index]?.data ?? history.data?.find((item) => item.id === id);
    return task ? [task] : [];
  });
  const run = useMutation({
    mutationFn: (input: TerminalContinuityRequest) => api.runTerminalContinuity(input),
    onSuccess: (task) => {
      client.setQueryData<Task>(["terminal-continuity", "task", task.id], task);
      setCreatedIDs((current) => [...new Set([...current, task.id])]);
      void client.invalidateQueries({ queryKey: historyKey });
      void client.invalidateQueries({ queryKey: ["tasks"] });
    },
    onError: (error) => notifyOperationError(error, "终端续接检测启动失败"),
  });
  return {
    history,
    run,
    tasks,
    results: terminalContinuityResults(tasks),
    busyIDs: terminalContinuityBusyIDs(tasks),
    activeTask: tasks.find(isTerminalContinuityActive),
    taskError: queries.some((query) => query.isError),
    taskFetching: queries.some((query) => query.isFetching),
    retryTasks: (): void => {
      for (const query of queries) if (query.isError) void query.refetch();
    },
  };
}
