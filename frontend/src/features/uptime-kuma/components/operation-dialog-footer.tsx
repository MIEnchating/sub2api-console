import type { ReactNode } from "react";
import type { Task } from "@/api";
import { TaskProgressState } from "@/components/task-startup-state";
import { DialogFooter } from "@/components/ui/dialog";

export function OperationDialogFooter(props: {
  pending: boolean;
  task?: Pick<Task, "message" | "progress"> | null;
  children: ReactNode;
}) {
  return (
    <DialogFooter className="flex-col sm:flex-col">
      {props.pending && props.task && (
        <TaskProgressState message={props.task.message} progress={props.task.progress} />
      )}
      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">{props.children}</div>
    </DialogFooter>
  );
}
