import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type TaskConcurrencySettings } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { ContentRetry } from "@/components/content-retry";
import { notifyOperationError } from "@/lib/operation-feedback";
import { SettingsFooter } from "./settings-footer";
import { taskPoolLabels } from "../constants";
import { taskConcurrencySchema, type TaskConcurrencyValues } from "../lib/task-concurrency-schema";

function formValues(settings: TaskConcurrencySettings): TaskConcurrencyValues {
  return {
    limits: Object.fromEntries(settings.pools.map((pool) => [pool.id, String(pool.limit)])),
    queueCapacity: String(settings.queue_capacity),
    version: settings.version,
  };
}

export function TaskConcurrencySettingsCard() {
  const client = useQueryClient();
  const settings = useQuery({
    queryKey: ["task-concurrency"],
    queryFn: api.taskConcurrency,
    refetchInterval: 10000,
  });
  const form = useForm<TaskConcurrencyValues>({
    resolver: zodResolver(taskConcurrencySchema),
    defaultValues: { limits: {}, queueCapacity: "", version: "" },
  });
  const save = useMutation({
    mutationFn: (values: TaskConcurrencyValues) =>
      api.updateTaskConcurrency({
        limits: Object.fromEntries(
          Object.entries(values.limits).map(([id, value]) => [id, Number(value)]),
        ),
        queue_capacity: Number(values.queueCapacity),
        version: values.version,
      }),
    onSuccess: (value) => {
      client.setQueryData(["task-concurrency"], value);
      form.reset(formValues(value));
      toast.success("任务并发设置已保存并生效");
    },
    onError: (error) => notifyOperationError(error, "任务并发设置保存失败"),
  });
  useEffect(() => {
    if (settings.data && !form.formState.isDirty) form.reset(formValues(settings.data));
  }, [settings.data, form, form.formState.isDirty]);

  if (!settings.data && settings.isPending)
    return (
      <Card
        size="sm"
        className="h-full min-h-0"
        role="status"
        aria-label="正在读取任务并发设置"
        aria-busy="true"
      >
        <CardHeader>
          <CardTitle>任务并发</CardTitle>
          <Skeleton className="h-4 w-2/3" />
        </CardHeader>
        <CardContent className="grid min-h-0 gap-4 overflow-auto sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 9 }, (_, index) => (
            <Skeleton key={index} className="h-24" />
          ))}
        </CardContent>
      </Card>
    );
  if (!settings.data)
    return <ContentRetry pending={settings.isFetching} onRetry={() => void settings.refetch()} />;

  return (
    <Card size="sm" className="h-full min-h-0 min-w-0">
      <CardHeader>
        <CardTitle>任务并发</CardTitle>
        <CardDescription>
          各模块独立排队，达到并发上限后等待，队列满时才拒绝新任务。这里控制后台任务，不修改上游账号并发额度。
        </CardDescription>
      </CardHeader>
      <form
        noValidate
        className="flex min-h-0 flex-1 flex-col"
        onSubmit={form.handleSubmit((values) => save.mutate(values))}
      >
        <CardContent className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
          <fieldset disabled={save.isPending} className="grid min-w-0 gap-4 py-3">
            <div className="flex flex-wrap items-center gap-3">
              <label htmlFor="task-queue-capacity">每个模块的等待队列容量</label>
              <Input
                id="task-queue-capacity"
                type="number"
                min={1}
                max={10000}
                className="w-28"
                aria-invalid={Boolean(form.formState.errors.queueCapacity)}
                {...form.register("queueCapacity")}
              />
              {form.formState.errors.queueCapacity && (
                <p role="alert" className="text-destructive text-sm">
                  {form.formState.errors.queueCapacity.message}
                </p>
              )}
            </div>
            <p className="text-muted-foreground text-sm">
              保存后立即生效，重启后保留；调低上限不会中断已有任务。运行和等待数量每 10
              秒更新。后台维护与自动检测调度包含常驻任务。
            </p>
            <div className="grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {settings.data.pools.map((pool) => (
                <div key={pool.id} className="grid min-w-0 gap-2 rounded-md border p-3">
                  <label htmlFor={`task-limit-${pool.id}`} className="font-medium">
                    {taskPoolLabels[pool.id] ?? pool.id}并发上限
                  </label>
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <Input
                      id={`task-limit-${pool.id}`}
                      type="number"
                      min={1}
                      max={10000}
                      className="w-28"
                      aria-invalid={Boolean(form.formState.errors.limits?.[pool.id])}
                      {...form.register(`limits.${pool.id}`)}
                    />
                    <span className="text-muted-foreground text-xs tabular-nums">
                      运行 {pool.running} · 等待 {pool.waiting} · 生效上限 {pool.limit}
                    </span>
                  </div>
                  {form.formState.errors.limits?.[pool.id] && (
                    <p role="alert" className="text-destructive text-sm">
                      {form.formState.errors.limits[pool.id]?.message}
                    </p>
                  )}
                </div>
              ))}
            </div>
          </fieldset>
        </CardContent>
        <SettingsFooter>
          <Button
            type="button"
            variant="outline"
            disabled={save.isPending || settings.isFetching}
            onClick={() => {
              form.reset(formValues(settings.data));
              void settings.refetch();
            }}
          >
            重新读取
          </Button>
          <Button type="submit" disabled={save.isPending || !form.formState.isDirty}>
            {save.isPending ? "保存中…" : "保存任务并发"}
          </Button>
        </SettingsFooter>
      </form>
    </Card>
  );
}
