import type { ReactNode } from "react";
import { DialogFooter } from "@/components/ui/dialog";
import { KumaTaskProgress, type KumaProgressTask } from "./kuma-task-progress";

export function OperationDialogFooter(props: {
  pending: boolean;
  starting?: boolean;
  task?: KumaProgressTask | null;
  children: ReactNode;
}) {
  return (
    <DialogFooter className="flex-col sm:flex-col">
      {props.pending && <KumaTaskProgress task={props.task} starting={props.starting} />}
      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">{props.children}</div>
    </DialogFooter>
  );
}
