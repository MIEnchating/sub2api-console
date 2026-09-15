import type { ReactElement } from "react";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { api, type SetupStatus, type WorkbenchScope } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { workbenchKeys } from "../constants";
import { WorkbenchSMSReceipts } from "./workbench-sms-receipts";

export function WorkbenchTaskOrders(props: { taskId: string }): ReactElement {
  const client = useQueryClient();
  const local = client.getQueryData<SetupStatus>(["setup-status"])?.target_configured === false;
  const scopes: WorkbenchScope[] = local ? ["local-export"] : ["managed", "local-export"];
  const queries = useQueries({
    queries: scopes.map((scope) => ({
      queryKey: [...workbenchKeys.smsReceipts, scope],
      queryFn: (context: { signal: AbortSignal }) =>
        api.workbenchSMSReceipts(context.signal, scope),
      retry: false,
      gcTime: 0,
    })),
  });
  const matches = scopes.filter((_scope, index) =>
    queries[index].data?.some((item) => item.task_id === props.taskId),
  );
  return (
    <div className="grid gap-3">
      {queries.some((query) => query.isPending) && (
        <ContentLoading label="正在读取本次授权的短信订单" compact />
      )}
      {queries.some((query) => query.isError) && (
        <ContentRetry
          pending={queries.some((query) => query.isFetching)}
          onRetry={() => {
            for (const query of queries) if (query.isError) void query.refetch();
          }}
        />
      )}
      {matches.map((scope) => (
        <WorkbenchSMSReceipts key={scope} scope={scope} taskId={props.taskId} />
      ))}
      {queries.every((query) => query.isSuccess) && !matches.length && (
        <p className="text-sm text-muted-foreground">本次授权没有短信订单</p>
      )}
    </div>
  );
}
