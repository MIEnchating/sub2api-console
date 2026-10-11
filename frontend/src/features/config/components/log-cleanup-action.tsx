import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api, type LogCleanupStatus } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { TaskCancelButton, TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";

export function LogCleanupAction(props: {
  retentionDays: number;
  ready: boolean;
  activeTaskId?: string;
}): ReactElement {
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [submittedTaskId, setTaskId] = useState<string | null>(null);
  const taskId = props.activeTaskId ?? submittedTaskId;
  useEffect(() => {
    if (props.activeTaskId) setTaskId(props.activeTaskId);
  }, [props.activeTaskId]);
  const notified = useRef<string | null>(null);
  const task = useQuery({
    queryKey: ["task", taskId],
    queryFn: ({ signal }) => api.task(taskId!, signal),
    enabled: taskId !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status === "succeeded" || status === "failed" || status === "cancelled"
        ? false
        : 1_000;
    },
  });
  const terminal =
    task.data?.status === "succeeded" ||
    task.data?.status === "failed" ||
    task.data?.status === "cancelled";
  const active = taskId !== null && !terminal;
  const displayedDays =
    active && typeof task.data?.result.retention_days === "number"
      ? task.data.result.retention_days
      : props.retentionDays;
  const valid =
    props.ready &&
    Number.isInteger(props.retentionDays) &&
    props.retentionDays >= 1 &&
    props.retentionDays <= 3650;
  const start = useMutation({
    mutationFn: () => api.clearLogs(props.retentionDays),
    onSuccess: (value) => {
      client.setQueryData(["task", value.id], value);
      setTaskId(value.id);
      client.setQueryData<LogCleanupStatus>(["log-cleanup"], (current) =>
        current ? { ...current, task_id: value.id } : current,
      );
      void client.invalidateQueries({ queryKey: ["log-cleanup"] });
    },
    onError: (error) => notifyOperationError(error, "日志清理启动失败"),
  });
  useEffect(() => {
    if (!terminal || !task.data || notified.current === task.data.id) return;
    notified.current = task.data.id;
    if (task.data.status === "succeeded") {
      toast.success(task.data.message);
      setOpen(false);
    } else if (task.data.status === "cancelled") {
      toast.info(task.data.message);
    } else {
      notifyOperationError(new Error(task.data.message), "日志清理未完成");
    }
    void Promise.all([
      client.invalidateQueries({ queryKey: ["logs"] }),
      client.invalidateQueries({ queryKey: ["log-cleanup"] }),
      client.invalidateQueries({ queryKey: ["overview"] }),
      client.invalidateQueries({ queryKey: ["tasks"] }),
      client.invalidateQueries({ queryKey: ["model-animation", "history"] }),
    ]);
  }, [client, task.data, terminal]);

  return (
    <>
      <Button
        variant="outline"
        data-testid="log-cleanup-clear"
        disabled={start.isPending || (!active && !valid)}
        onClick={() => setOpen(true)}
      >
        <Trash2 size={16} aria-hidden="true" />
        {active ? "查看清理进度" : "立即按期限清理"}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>按保留期限清理日志</DialogTitle>
            <DialogDescription>
              将删除 {displayedDays}{" "}
              天以前的任务、运行、事件和变更记录。期限内日志和执行中的任务会保留；账号、分组、策略、告警、健康样本和上游使用记录不受影响。清理在后台执行，关闭弹窗后仍会继续。
            </DialogDescription>
          </DialogHeader>
          {start.isPending && <TaskStartupState message="正在创建日志清理任务" />}
          {active && (task.data || !task.error) && (
            <TaskStartupState message={task.data?.message ?? "正在读取日志清理任务"} />
          )}
          {active && task.error && (
            <ContentRetry
              onRetry={() => {
                void task.refetch();
              }}
              pending={task.isFetching}
            />
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {active || start.isPending ? "关闭" : "取消"}
            </Button>
            {active && taskId ? (
              <TaskCancelButton taskId={taskId} />
            ) : (
              <Button
                variant="destructive"
                disabled={start.isPending || !valid}
                onClick={() => start.mutate()}
              >
                <Trash2 size={15} aria-hidden="true" />
                {start.isPending ? "创建中…" : "确认清理"}
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
