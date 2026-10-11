import { useCallback, useEffect, useState } from "react";
import { useQueryClient, type UseQueryResult } from "@tanstack/react-query";

import type { AccountStatus } from "@/api";

type ReadState = {
  taskId: string;
  attempt: number;
  failed: boolean;
};

/** 终态任务刷新账号投影后才显示结果；读取错误沿用全局 QueryCache 提示。 */
export function useConfigurationResults(
  completedTaskId: string | null,
  refetchAccounts: UseQueryResult<AccountStatus[]>["refetch"],
): { ready: boolean; failed: boolean; retry: () => void } {
  const client = useQueryClient();
  const [attempt, setAttempt] = useState(0);
  const [read, setRead] = useState<ReadState | null>(null);
  const retry = useCallback(() => setAttempt((current) => current + 1), []);

  useEffect(() => {
    if (!completedTaskId) return;
    let current = true;
    // 其他账号视图标记为过期，当前账号列表只执行一次明确的刷新。
    void client.invalidateQueries({ queryKey: ["accounts"], refetchType: "none" });
    void refetchAccounts().then((result) => {
      if (!current) return;
      setRead({ taskId: completedTaskId, attempt, failed: result.isError });
    });
    return () => {
      current = false;
    };
  }, [completedTaskId, attempt, client, refetchAccounts]);

  const settled =
    completedTaskId !== null && read?.taskId === completedTaskId && read.attempt === attempt;
  return {
    ready: settled && !read.failed,
    failed: settled && read.failed,
    retry,
  };
}
