import { useState, type ReactElement } from "react";
import { toast } from "sonner";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type DetectionTask } from "@/api";
import { Button } from "@/components/ui/button";
import { ContentRetry } from "@/components/content-retry";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { DetectionTaskEditor } from "./detection-task-editor";
import { DetectionTaskDetails } from "./detection-task-details";

export function DetectionTaskPanel(): ReactElement {
  const client = useQueryClient();
  const plans = useQuery({
    queryKey: ["model-detection-tasks"],
    queryFn: api.detectionTasks,
    refetchInterval: 5000,
  });
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const [edit, setEdit] = useState<DetectionTask | null | undefined>(undefined);
  const [remove, setRemove] = useState<DetectionTask | null>(null);
  const [details, setDetails] = useState<string | null>(null);
  const run = useMutation({
    mutationFn: (value: DetectionTask) => api.runDetectionTask(value.id, value.version),
    onSuccess: () => {
      toast.success("检测任务已启动，可点击运行详情查看进度");
      void client.invalidateQueries({ queryKey: ["model-detection-tasks"] });
      void client.invalidateQueries({ queryKey: ["model-animation"] });
      void client.invalidateQueries({ queryKey: ["terminal-continuity", "history"] });
    },
    onError: (error) => notifyOperationError(error, "检测任务启动失败"),
  });
  const deletion = useMutation({
    mutationFn: (value: DetectionTask) => api.deleteDetectionTask(value.id, value.version),
    onSuccess: (values) => {
      client.setQueryData(["model-detection-tasks"], values);
      setRemove(null);
    },
    onError: (error) => notifyOperationError(error, "检测任务删除失败"),
  });
  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b p-3">
        <p className="text-sm text-muted-foreground">
          按分组保存检测流程，支持手动执行与自动检测。
        </p>
        <Button onClick={() => setEdit(null)}>新增任务</Button>
      </header>
      <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-3" aria-label="检测任务列表">
        {plans.isPending && (
          <div role="status" aria-label="正在读取检测任务" className="space-y-3">
            {[0, 1, 2].map((index) => (
              <div key={index} className="h-32 animate-pulse rounded border bg-muted" />
            ))}
          </div>
        )}
        {plans.isError && (
          <ContentRetry onRetry={() => void plans.refetch()} pending={plans.isFetching} />
        )}
        {plans.isSuccess && plans.data.length === 0 && (
          <p className="py-8 text-center text-sm text-muted-foreground">
            尚无检测任务，点击“新增任务”配置分组和检测流程。
          </p>
        )}
        {plans.data?.map((plan) => (
          <article
            key={plan.id}
            aria-label={`检测任务 ${plan.name}`}
            className="space-y-2 rounded-lg border p-4"
          >
            <div className="flex flex-wrap items-start justify-between gap-3">
              <h3 className="min-w-0 font-medium wrap-anywhere">{plan.name}</h3>
              <span className="text-xs text-muted-foreground">
                {plan.running ? "执行中" : "未在执行"}
              </span>
            </div>
            <p className="text-sm wrap-anywhere">
              分组：
              {plan.group_ids
                .map((id) => groups.data?.find((group) => group.id === id)?.name ?? `ID ${id}`)
                .join("、")}
            </p>
            <p className="text-sm wrap-anywhere">
              模型：{plan.model} · {plan.precheck ? "前置检测 → " : ""}
              {plan.terminal ? `终端检测 ${plan.terminal_rounds} 轮 → ` : ""}动画检测
            </p>
            <p className="text-xs text-muted-foreground wrap-anywhere">
              {plan.automatic ? "自动检测已开启" : "自动检测已关闭"}
              {plan.next_at
                ? ` · 下次 ${new Date(plan.next_at).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" })}（北京时间）`
                : ""}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button disabled={plan.running || run.isPending} onClick={() => run.mutate(plan)}>
                立即执行
              </Button>
              <Button variant="outline" onClick={() => setEdit(plan)}>
                编辑
              </Button>
              <Button
                variant="outline"
                disabled={!plan.last_task_id}
                onClick={() => setDetails(plan.last_task_id ?? null)}
              >
                运行详情
              </Button>
              <Button
                variant="outline"
                disabled={plan.running || deletion.isPending}
                onClick={() => setRemove(plan)}
              >
                删除
              </Button>
            </div>
          </article>
        ))}
      </div>
      {edit !== undefined && (
        <DetectionTaskEditor value={edit ?? undefined} onClose={() => setEdit(undefined)} />
      )}
      {details && <DetectionTaskDetails id={details} onClose={() => setDetails(null)} />}
      <ConfirmActionDialog
        open={remove !== null}
        title="删除检测任务"
        description={`删除“${remove?.name ?? ""}”并停止后续自动检测，已完成的运行记录保留。`}
        confirmLabel="确认删除"
        pending={deletion.isPending}
        onOpenChange={(open) => {
          if (!open) setRemove(null);
        }}
        onConfirm={() => {
          if (remove) deletion.mutate(remove);
        }}
      />
    </div>
  );
}
