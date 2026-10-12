import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { api, type UpstreamExclusionRequest } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { TaskProgressState, TaskStartupState } from "@/components/task-startup-state";
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
import { exclusionSchema, type ExclusionForm } from "../lib/upstream-exclusion-schema";

type Props = { host: string; excluded: boolean };
const refreshKeys = [
  "upstreams",
  "accounts",
  "groups",
  "routing",
  "tasks",
  "logs",
  "upstream-exclusion-preview",
];

export function UpstreamExclusionDialog(
  props: Props & {
    open: boolean;
    stopOnly: boolean;
    onActiveChange: (active: boolean) => void;
    onClose: () => void;
  },
) {
  const client = useQueryClient();
  const submitted = useRef(false);
  const notified = useRef<string | null>(null);
  const [taskId, setTaskId] = useState<string | null>(null);
  const form = useForm<ExclusionForm>({
    resolver: zodResolver(exclusionSchema),
    defaultValues: { stopScheduling: props.stopOnly },
  });
  const preview = useQuery({
    queryKey: ["upstream-exclusion-preview", props.host],
    queryFn: ({ signal }) => api.upstreamExclusionPreview(props.host, signal),
    staleTime: 0,
    enabled: props.open && taskId === null,
  });
  const mutation = useMutation({
    mutationFn: (input: UpstreamExclusionRequest) => api.setUpstreamExclusion(props.host, input),
    retry: false,
    onSuccess: (task) => {
      setTaskId(task.id);
      client.setQueryData(["upstream-exclusion-task", task.id], task);
    },
    onError: (error) => {
      submitted.current = false;
      notifyOperationError(error, "上游排除设置失败");
      void preview.refetch();
    },
  });
  const task = useQuery({
    queryKey: ["upstream-exclusion-task", taskId],
    queryFn: ({ signal }) => api.task(taskId!, signal),
    enabled: taskId !== null,
    refetchInterval: (query) =>
      query.state.data && ["queued", "running"].includes(query.state.data.status) ? 1000 : false,
  });
  const finished = !!task.data && !["queued", "running"].includes(task.data.status);
  useEffect(() => {
    props.onActiveChange(mutation.isPending || (taskId !== null && !finished));
  }, [props.onActiveChange, mutation.isPending, taskId, finished]);
  useEffect(() => {
    if (!finished || !task.data || notified.current === task.data.id) return;
    notified.current = task.data.id;
    void Promise.all(refreshKeys.map((key) => client.invalidateQueries({ queryKey: [key] })));
    if (task.data.status !== "succeeded")
      notifyOperationError(
        new Error(
          typeof task.data.result.error === "string" ? task.data.result.error : task.data.message,
        ),
        "上游排除任务未完成",
      );
  }, [client, finished, task.data]);
  const restoring = props.excluded && !props.stopOnly;
  let title = restoring ? "取消排除上游" : "排除上游";
  let confirmLabel = restoring ? "确认取消排除" : "确认排除";
  if (props.stopOnly) {
    title = "关闭已排除上游的账号调度";
    confirmLabel = "确认关闭调度";
  }
  const rows = Array.isArray(task.data?.result.accounts) ? task.data.result.accounts : [];
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {restoring
              ? "取消排除后重新纳入管控，本次操作不会直接开启账号调度。"
              : "排除后停止对该上游及其账号的自动调度、倍率维护、探活和鉴权恢复等管控。请选择是否同时关闭所有关联账号的调度。"}
          </DialogDescription>
        </DialogHeader>
        <p className="break-all text-sm">{props.host}</p>
        {!taskId && (
          <>
            {preview.isPending && <ContentLoading label="正在读取受影响账号" />}
            {!preview.data && preview.isError && (
              <ContentRetry
                onRetry={() => {
                  void preview.refetch();
                }}
                pending={preview.isFetching}
              />
            )}
            {preview.data && (
              <div className="grid min-w-0 gap-2 text-sm">
                <p>关联账号：{preview.data.account_count} 个</p>
                <ul aria-label="受影响账号" className="max-h-48 overflow-auto break-all">
                  {preview.data.accounts.map((account) => (
                    <li key={account.id}>
                      {account.name}（ID：{account.id}）
                    </li>
                  ))}
                </ul>
                {preview.data.account_count === 0 && (
                  <p>当前没有关联账号，之后关联的账号也会遵循上游排除状态。</p>
                )}
              </div>
            )}
            <form
              id="upstream-exclusion-form"
              onSubmit={form.handleSubmit((values) => {
                if (submitted.current || !preview.data || preview.isFetching) return;
                submitted.current = true;
                mutation.mutate({
                  upstream_id: preview.data.upstream_id,
                  excluded: !restoring,
                  stop_scheduling: !restoring && values.stopScheduling,
                  expected_account_ids: preview.data.account_ids,
                });
              })}
            >
              {!restoring && (
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    {...form.register("stopScheduling")}
                    disabled={mutation.isPending}
                  />
                  同时关闭该上游下所有账号的调度
                </label>
              )}
              {!restoring && (
                <p className="mt-2 text-sm text-muted-foreground">
                  不勾选时保留账号当前调度状态；勾选后包含手动控制账号，执行需要完全模式。
                </p>
              )}
            </form>
          </>
        )}
        {mutation.isPending && <TaskStartupState message="正在创建上游排除任务" />}
        {taskId && !task.data && !task.isError && <TaskStartupState message="正在读取任务状态" />}
        {taskId && task.isError && (
          <ContentRetry
            onRetry={() => {
              void task.refetch();
            }}
            pending={task.isFetching}
          />
        )}
        {task.data?.status === "queued" && <TaskStartupState message={task.data.message} />}
        {task.data && task.data.status === "running" && (
          <TaskProgressState
            message={task.data.message}
            progress={task.data.progress}
            taskId={task.data.id}
          />
        )}
        {finished && task.data && (
          <div role="status" className="grid gap-2 text-sm">
            <p>{task.data.message}</p>
            <ul aria-label="账号调度结果" className="max-h-48 overflow-auto break-all">
              {rows.map((row: unknown) => {
                if (!row || typeof row !== "object" || !("account_id" in row)) return null;
                const id = String(row.account_id);
                return (
                  <li key={id}>
                    账号 {id}：{"error" in row ? String(row.error) : "已确认停止调度"}
                  </li>
                );
              })}
            </ul>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            {taskId ? "关闭" : "取消"}
          </Button>
          {!taskId && (
            <Button
              type="submit"
              form="upstream-exclusion-form"
              disabled={!preview.data || preview.isFetching || mutation.isPending}
            >
              {confirmLabel}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
