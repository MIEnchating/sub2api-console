import { useState, type ReactElement } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { ScanSearch, LoaderCircle } from "lucide-react";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ContentRetry } from "@/components/content-retry";
import { TaskStartupState } from "@/components/task-startup-state";
import { notifyOperationError } from "@/lib/operation-feedback";
import { animationResultSchema } from "@/features/model-check/lib/animation-task-results";
import { PrecheckAccountResult } from "@/features/model-check/components/precheck-account-result";
import type { WorkbenchAccount } from "../types";

export function AccountCheckButton(props: { account: WorkbenchAccount }): ReactElement {
  const [open, setOpen] = useState(false);
  const start = useMutation({
    mutationFn: async () => {
      const settings = await api.workbenchImportModel();
      return api.runAnimation({
        targets: [{ account_id: props.account.id, model: settings.model }],
        mode: "precheck",
        precheck_questions: ["candy"],
        timeout_seconds: 120,
      });
    },
    onSuccess: () => setOpen(true),
    onError: (error) => notifyOperationError(error, "智商检测启动失败"),
  });
  const detail = useQuery({
    queryKey: ["account-workbench", "check", start.data?.id],
    queryFn: ({ signal }) => api.task(start.data!.id, signal),
    enabled: !!start.data,
    refetchInterval: (query) =>
      !query.state.data || ["queued", "running"].includes(query.state.data.status) ? 1000 : false,
  });
  const task = detail.data ?? start.data;
  const running = start.isPending || (!!task && ["queued", "running"].includes(task.status));
  const rows: unknown[] = Array.isArray(task?.result.animations) ? task.result.animations : [];
  const result = rows
    .map((row) => animationResultSchema.safeParse(row))
    .find((row) => row.success && row.data.account_id === props.account.id);
  return (
    <>
      <TableActionButton
        label={running ? "检测中…" : "智商检测"}
        disabled={running}
        ariaLabel={`智商检测 ${props.account.name || props.account.id}`}
        onClick={() => {
          setOpen(true);
          start.mutate();
        }}
      >
        {running ? (
          <LoaderCircle aria-hidden="true" className="animate-spin" />
        ) : (
          <ScanSearch aria-hidden="true" />
        )}
      </TableActionButton>
      {open && (
        <Dialog open onOpenChange={setOpen}>
          <DialogContent width="wide">
            <DialogHeader>
              <DialogTitle>智商检测 · {props.account.name || props.account.id}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              {running && <TaskStartupState message="正在执行糖果题检测" />}
              {detail.isError && (
                <ContentRetry pending={detail.isFetching} onRetry={() => void detail.refetch()} />
              )}
              {result?.success && !running && (
                <PrecheckAccountResult result={result.data} status={task?.status} />
              )}
              {!running && !result?.success && task && (
                <p className="text-sm">{task.message || "检测未返回结果"}</p>
              )}
              {start.isError && (
                <ContentRetry pending={start.isPending} onRetry={() => start.mutate()} />
              )}
            </DialogBody>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
