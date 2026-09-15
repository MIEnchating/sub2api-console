import { useState, type ReactElement } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type Task } from "@/api";
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
import { taskIsTerminal } from "@/lib/task-state";
import { taskStatusLabels, workbenchKeys } from "../constants";
import { WorkbenchGlobalCancel } from "./workbench-global-cancel";

type Selection = { mode: "delete" | "cancel"; tasks: Task[] };

export function WorkbenchHistoryActions(props: {
  tasks: Task[];
  checked: Set<string>;
  onDeleted: (ids: string[]) => void;
  hideGlobal?: boolean;
}): ReactElement {
  const client = useQueryClient();
  const [selection, setSelection] = useState<Selection | null>(null);
  const selected = props.tasks.filter((task) => props.checked.has(task.id));
  const deletable = selected.filter(taskIsTerminal);
  const cancellable = selected.filter((task) => !taskIsTerminal(task));
  const action = useMutation({
    mutationFn: async (value: Selection): Promise<void> => {
      const items = value.tasks.map((task) => ({ id: task.id, updated_at: task.updated_at }));
      if (value.mode === "delete") {
        await api.deleteWorkbenchHistory(items);
        props.onDeleted(items.map((item) => item.id));
        toast.success(`已删除 ${items.length} 条处理记录`);
      } else {
        const result = await api.cancelWorkbenchHistory(items);
        const count = result.items.filter((item) => item.cancelled).length;
        toast.success(`已请求取消 ${count} 个任务，其余任务已结束或不在当前进程运行`);
      }
    },
    onSuccess: () => {
      setSelection(null);
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
      void client.invalidateQueries({ queryKey: ["account-workbench", "task"] });
    },
    onError: (error) => notifyOperationError(error, "处理记录操作失败，请刷新后重试"),
  });
  return (
    <>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={!deletable.length || action.isPending}
          onClick={() => setSelection({ mode: "delete", tasks: deletable })}
        >
          删除选中记录（{deletable.length}）
        </Button>
        <Button
          variant="outline"
          disabled={!cancellable.length || action.isPending}
          onClick={() => setSelection({ mode: "cancel", tasks: cancellable })}
        >
          取消选中任务（{cancellable.length}）
        </Button>
        {!props.hideGlobal && <WorkbenchGlobalCancel disabled={action.isPending} />}
      </div>
      <Dialog
        open={selection !== null}
        onOpenChange={(open) => {
          if (!open && !action.isPending) setSelection(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {selection?.mode === "delete" ? "确认删除处理记录" : "确认取消处理任务"}
            </DialogTitle>
            <DialogDescription>
              {selection?.mode === "delete"
                ? "仅删除下列已结束的处理记录。线上账号、私有结果文件和独立审计记录会保留。"
                : "停止下列任务的后续步骤。正在提交的操作可能已经生效，请在任务结束后核对结果。"}
            </DialogDescription>
          </DialogHeader>
          <ul className="max-h-64 space-y-2 overflow-y-auto text-sm" aria-label="操作影响范围">
            {selection?.tasks.map((task) => (
              <li key={task.id} className="wrap-anywhere">
                {task.message || task.id} · {taskStatusLabels[task.status]}
                <span className="block text-xs text-muted-foreground">ID：{task.id}</span>
              </li>
            ))}
          </ul>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={action.isPending}
              onClick={() => setSelection(null)}
            >
              返回
            </Button>
            <Button
              variant="destructive"
              disabled={!selection || action.isPending}
              onClick={() => {
                if (selection) action.mutate(selection);
              }}
            >
              {action.isPending ? "正在处理…" : "确认执行"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
