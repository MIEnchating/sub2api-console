import { useState, type ReactElement } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Task } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskStatusLabels, workbenchKeys } from "../constants";

export function WorkbenchGlobalCancel(props: { disabled?: boolean }): ReactElement {
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  return (
    <>
      <Button variant="outline" disabled={props.disabled} onClick={() => setOpen(true)}>
        取消全部工作台任务
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!pending) setOpen(value);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认取消全部工作台任务</DialogTitle>
            <DialogDescription>
              下列范围包含全部历史中的活动任务。确认后停止后续步骤；正在提交的操作可能已经生效，请在结束后核对结果。预览后新建的任务不在本次范围内。
            </DialogDescription>
          </DialogHeader>
          {open && <GlobalCancelScope onClose={() => setOpen(false)} onPending={setPending} />}
        </DialogContent>
      </Dialog>
    </>
  );
}

function GlobalCancelScope(props: {
  onClose: () => void;
  onPending: (value: boolean) => void;
}): ReactElement {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: [...workbenchKeys.history, "active"],
    queryFn: api.activeWorkbenchHistory,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const cancel = useMutation({
    mutationFn: (tasks: Task[]) =>
      api.cancelWorkbenchHistory(
        tasks.map((task) => ({ id: task.id, updated_at: task.updated_at })),
      ),
    onMutate: () => props.onPending(true),
    onSuccess: (result) => {
      toast.success(
        `已请求取消 ${result.items.filter((item) => item.cancelled).length} 个工作台任务`,
      );
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      void client.invalidateQueries({ queryKey: ["account-workbench", "task"] });
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "批量取消失败，请刷新范围后重试"),
    onSettled: () => props.onPending(false),
  });
  return (
    <>
      <DialogBody className="grid gap-3">
        {query.isPending && <ContentLoading label="正在读取全部活动任务" />}
        {query.isError && (
          <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
        )}
        {query.data && (
          <>
            <p className="text-sm">共 {query.data.length} 个活动任务</p>
            <ul className="max-h-64 space-y-2 overflow-y-auto text-sm" aria-label="全部取消范围">
              {query.data.map((task) => (
                <li key={task.id} className="wrap-anywhere">
                  {task.message || task.id} · {taskStatusLabels[task.status]}
                  <span className="block text-xs text-muted-foreground">ID：{task.id}</span>
                </li>
              ))}
            </ul>
          </>
        )}
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" disabled={cancel.isPending} onClick={props.onClose}>
          返回
        </Button>
        <Button
          variant="destructive"
          disabled={query.isFetching || query.isError || !query.data?.length || cancel.isPending}
          onClick={() => {
            if (query.data) cancel.mutate(query.data);
          }}
        >
          {cancel.isPending ? "正在请求取消…" : "确认取消这些任务"}
        </Button>
      </DialogFooter>
    </>
  );
}
