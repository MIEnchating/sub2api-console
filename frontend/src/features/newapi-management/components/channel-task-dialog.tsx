import type { Task } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { TaskProgressState, TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export const channelTaskFinished = (task: Task | null | undefined): boolean =>
  !!task && ["succeeded", "partial", "failed", "cancelled"].includes(task.status);

function taskItems(task: Task): Array<{ channel_id: string; status: string; message: string }> {
  if (!Array.isArray(task.result.items)) return [];
  return task.result.items.filter(
    (item): item is { channel_id: string; status: string; message: string } =>
      typeof item === "object" &&
      item !== null &&
      typeof item.channel_id === "string" &&
      typeof item.status === "string" &&
      typeof item.message === "string",
  );
}

export function ChannelTaskDialog(props: {
  task: Task;
  open: boolean;
  error: boolean;
  retrying: boolean;
  onRetry: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  const finished = channelTaskFinished(props.task);
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent width="progress" height="large">
        <DialogHeader>
          <DialogTitle>渠道批量操作</DialogTitle>
          <DialogDescription>
            任务 ID {props.task.id}，关闭弹窗后仍在后台执行，可在日志中心查看。
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid gap-3">
          {props.error && <ContentRetry pending={props.retrying} onRetry={props.onRetry} />}
          {!finished && props.task.progress === 0 && (
            <TaskStartupState message={props.task.message} />
          )}
          {!finished && props.task.progress > 0 && (
            <TaskProgressState message={props.task.message} progress={props.task.progress} />
          )}
          {finished && (
            <p role="status" className="text-sm">
              {props.task.message}
            </p>
          )}
          <div
            className="max-h-72 overflow-y-auto rounded-md border"
            aria-label="渠道执行结果"
            role="group"
          >
            {taskItems(props.task).map((item) => (
              <div key={item.channel_id} className="grid gap-1 border-b p-3 last:border-b-0">
                <p className="text-sm font-medium">
                  渠道 {item.channel_id} · {item.status === "succeeded" ? "成功" : "未完成"}
                </p>
                <p className="text-muted-foreground break-all text-xs">{item.message}</p>
              </div>
            ))}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
