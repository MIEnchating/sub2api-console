import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { taskPollInterval } from "@/lib/task-state";
import { ModelCheckResult } from "./model-check-result";
import { modelCheckDialogLayout } from "./regular-check-panel";

export function ModelCheckHistoryResult(props: {
  taskID: string | null;
  onClose: () => void;
}): ReactElement {
  const task = useQuery({
    queryKey: ["model-check-task", props.taskID],
    queryFn: ({ signal }) => api.task(props.taskID!, signal),
    enabled: props.taskID !== null,
    refetchInterval: (query) => taskPollInterval(query, 2000),
  });
  const layout = modelCheckDialogLayout(task.data);
  return (
    <Dialog
      open={props.taskID !== null}
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent
        width={layout.width}
        height="tall"
        className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>历史检测结果</DialogTitle>
        </DialogHeader>
        <DialogBody className={layout.resultsReady ? "overflow-hidden pr-0" : undefined}>
          {!task.data && task.isPending ? <ContentLoading label="正在读取检测结果" /> : null}
          {task.isError ? (
            <ContentRetry pending={task.isFetching} onRetry={() => void task.refetch()} />
          ) : null}
          {task.data ? <ModelCheckResult task={task.data} /> : null}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
