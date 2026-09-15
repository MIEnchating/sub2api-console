import type { ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { taskPollInterval } from "@/lib/task-state";
import { workbenchKeys } from "../constants";
import { workbenchResultItems } from "../lib/task-results";
import { WorkbenchTaskResults } from "./workbench-task-results";

export function WorkbenchMaintenanceResult(props: { id?: string }): ReactElement {
  const query = useQuery({
    queryKey: workbenchKeys.task(props.id ?? ""),
    queryFn: () => api.task(props.id!),
    enabled: !!props.id,
    refetchInterval: taskPollInterval,
  });
  if (!props.id)
    return <p className="py-6 text-center text-sm text-muted-foreground">暂无维护结果</p>;
  if (query.isPending) return <ContentLoading label="正在读取最近维护结果" />;
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  if (query.data.operation !== "account-workbench-maintenance")
    return <p className="text-sm text-muted-foreground">最近维护记录不可用</p>;
  const items = workbenchResultItems(query.data.result);
  return (
    <section aria-label="最近维护结果" className="grid min-w-0 gap-2 border-t pt-3">
      <h3 className="text-sm font-medium">最近维护结果</h3>
      <p className="text-sm text-muted-foreground wrap-anywhere">{query.data.message}</p>
      <WorkbenchTaskResults items={items} />
    </section>
  );
}
