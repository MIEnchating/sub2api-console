import { useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { FileSearch } from "lucide-react";
import { api, type WorkbenchPendingUpload } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { RefreshButton } from "@/components/refresh-button";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { maintenanceUploadStatusLabels, workbenchKeys } from "../constants";
import { WorkbenchTask } from "./workbench-task";

export function WorkbenchMaintenanceUploads(props: {
  items: WorkbenchPendingUpload[];
  refreshing: boolean;
  onRefresh: () => void;
}): ReactElement {
  const [selected, setSelected] = useState<string | null>(null);
  const task = useQuery({
    queryKey: workbenchKeys.task(selected ?? ""),
    queryFn: () => api.task(selected!),
    enabled: selected !== null,
    gcTime: 0,
  });
  return (
    <section aria-label="维护待上传账号" className="grid min-w-0 gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-base font-medium">待上传账号（{props.items.length}）</h2>
        <RefreshButton
          ariaLabel="刷新待上传账号"
          pending={props.refreshing}
          onClick={props.onRefresh}
        />
      </div>
      {props.items.length === 0 && <p className="text-sm text-muted-foreground">暂无待上传账号</p>}
      <ul className="min-w-0 divide-y" aria-label="待上传账号列表">
        {props.items.map((item) => (
          <li key={item.id} className="flex min-w-0 flex-col gap-3 py-3 sm:flex-row sm:items-start">
            <div className="min-w-0 flex-1 space-y-1 text-sm wrap-anywhere">
              <p className="font-medium">{item.email || `账号 ${item.account_id}`}</p>
              <p className="text-muted-foreground">账号 ID：{item.account_id}</p>
              <p>{item.message}</p>
              {item.next_retry_at && (
                <p className="text-xs text-muted-foreground">
                  下次核对：<time dateTime={item.next_retry_at}>{item.next_retry_at}</time>
                </p>
              )}
              <p className="text-xs text-muted-foreground">
                保留至：<time dateTime={item.expires_at}>{item.expires_at}</time>
              </p>
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-2">
              <Badge variant={item.status === "review" ? "destructive" : "secondary"}>
                {maintenanceUploadStatusLabels[item.status]}
              </Badge>
              <Button
                variant="outline"
                aria-label={`查看账号 ${item.account_id} 的来源任务`}
                onClick={() => setSelected(item.source_task_id)}
              >
                <FileSearch aria-hidden="true" />
                来源任务
              </Button>
            </div>
          </li>
        ))}
      </ul>
      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      >
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>待上传来源任务</DialogTitle>
          </DialogHeader>
          {task.isPending && <ContentLoading label="正在读取待上传来源任务" />}
          {task.isError && (
            <ContentRetry pending={task.isFetching} onRetry={() => void task.refetch()} />
          )}
          {task.data && <WorkbenchTask key={task.data.id} task={task.data} />}
        </DialogContent>
      </Dialog>
    </section>
  );
}
