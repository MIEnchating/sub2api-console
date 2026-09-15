import type { ReactElement } from "react";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { api, type Task, type SetupStatus, type WorkbenchScope } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { workbenchKeys } from "../constants";
import { WorkbenchMixed } from "./workbench-mixed";

export function WorkbenchTaskRecovery(props: { task: Task }): ReactElement {
  const client = useQueryClient();
  const local = client.getQueryData<SetupStatus>(["setup-status"])?.target_configured === false;
  const scopes: WorkbenchScope[] = local ? ["local-export"] : ["managed", "local-export"];
  const queries = useQueries({
    queries: scopes.map((scope) => ({
      queryKey: [...workbenchKeys.queueRecoveries, scope],
      queryFn: (context: { signal: AbortSignal }) =>
        api.workbenchQueueRecoveries(context.signal, scope),
      retry: false,
      gcTime: 0,
    })),
  });
  const queryIndex = queries.findIndex((query) =>
    query.data?.some(
      (item) =>
        item.kind === "mixed" &&
        item.task_id === props.task.id &&
        item.id === props.task.result.recovery_id,
    ),
  );
  if (queryIndex >= 0)
    return <WorkbenchMixed scope={scopes[queryIndex]} recoveryTaskId={props.task.id} />;
  if (queries.some((query) => query.isPending))
    return <ContentLoading label="正在读取本批恢复资料" compact />;
  if (queries.some((query) => query.isError))
    return (
      <ContentRetry
        pending={queries.some((query) => query.isFetching)}
        onRetry={() => {
          for (const query of queries) void query.refetch();
        }}
      />
    );
  return <p className="text-sm text-muted-foreground">本批恢复资料已过期或已清除</p>;
}
