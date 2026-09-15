import { useEffect, useState, type ReactElement } from "react";
import { RotateCcw } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { api, type GroupStatus, type Task, type WorkbenchMaintenance } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { WorkbenchMaintenanceSkeleton } from "./workbench-page-skeletons";
import { FormField } from "@/components/form-field";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";
import { maintenanceSchema, type MaintenanceValues } from "../lib/schemas";
import { WorkbenchGroupPicker } from "./group-picker";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchMaintenanceAuthorization } from "./workbench-maintenance-authorization";
import { WorkbenchMaintenanceUploads } from "./workbench-maintenance-uploads";

export function WorkbenchMaintenancePanel(): ReactElement {
  const query = useQuery({
    queryKey: workbenchKeys.maintenance,
    queryFn: api.workbenchMaintenance,
    refetchInterval: (state) => {
      if (state.state.error) return false;
      return state.state.data?.enabled || state.state.data?.pending_uploads?.length ? 5000 : false;
    },
  });
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  if (query.isPending || groups.isPending) return <WorkbenchMaintenanceSkeleton />;
  if (!query.data || !groups.data)
    return (
      <ContentRetry
        pending={query.isFetching || groups.isFetching}
        onRetry={() => {
          void query.refetch();
          void groups.refetch();
        }}
      />
    );
  return (
    <div className="grid min-w-0 gap-4">
      <MaintenanceForm config={query.data} groups={groups.data} />
      <WorkbenchMaintenanceUploads
        items={query.data.pending_uploads ?? []}
        refreshing={query.isFetching}
        onRefresh={() => void query.refetch()}
      />
    </div>
  );
}

function MaintenanceForm(props: {
  config: WorkbenchMaintenance;
  groups: GroupStatus[];
}): ReactElement {
  const client = useQueryClient();
  const [baseline, setBaseline] = useState(props.config);
  const [confirm, setConfirm] = useState<MaintenanceValues | "check" | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const form = useForm<MaintenanceValues>({
    resolver: zodResolver(maintenanceSchema),
    defaultValues: props.config,
  });
  const save = useMutation({
    mutationFn: (value: MaintenanceValues) =>
      api.saveWorkbenchMaintenance({ ...value, revision: baseline.revision }),
    onSuccess: (value) => {
      setConfirm(null);
      setBaseline(value);
      form.reset(value);
      toast.success("账号维护设置已保存");
      client.setQueryData(workbenchKeys.maintenance, value);
    },
    onError: (error) => {
      notifyOperationError(error, "账号维护设置保存失败，请重新读取后重试");
      void client.invalidateQueries({ queryKey: workbenchKeys.maintenance });
    },
  });
  const check = useMutation({
    mutationFn: () => api.checkWorkbenchMaintenance(baseline.revision),
    onSuccess: (value) => {
      setConfirm(null);
      setTask(value);
      void client.invalidateQueries({ queryKey: workbenchKeys.history });
    },
    onError: (error) => notifyOperationError(error, "维护任务创建失败，请重新读取设置后重试"),
  });
  const pending = save.isPending || check.isPending;
  useEffect(() => {
    if (
      pending ||
      confirm !== null ||
      form.formState.isDirty ||
      baseline.revision === props.config.revision
    )
      return;
    setBaseline(props.config);
    form.reset(props.config);
  }, [baseline.revision, confirm, form, form.formState.isDirty, pending, props.config]);
  const confirmedSettings = confirm === "check" || confirm === null ? baseline : confirm;
  const scope = confirmedSettings.group_ids.length
    ? `分组 ID：${confirmedSettings.group_ids.join("、")}`
    : "全部 OpenAI OAuth 账号";
  const detection = confirmedSettings.check_after_repair
    ? `修复后使用 ${confirmedSettings.model} 检测，会产生模型调用用量。`
    : "修复后不执行模型检测。";
  return (
    <div className="grid gap-4">
      <form
        className="grid gap-4 rounded-lg border bg-card p-4"
        onSubmit={form.handleSubmit((value) => {
          if (value.enabled) setConfirm(value);
          else save.mutate(value);
        })}
      >
        <h2 className="font-medium">自动维护设置</h2>
        <p className="text-sm text-muted-foreground">
          定期检查账号登录状态，对需要恢复的账号执行修复。同一账号在冷却期内不会重复修复。分组留空时覆盖全部
          OpenAI OAuth 账号。
        </p>
        <label className="flex items-center gap-2 text-sm">
          <Controller
            control={form.control}
            name="enabled"
            render={({ field }) => (
              <Checkbox checked={field.value} onCheckedChange={field.onChange} disabled={pending} />
            )}
          />
          启用自动维护
        </label>
        <label className="flex items-center gap-2 text-sm">
          <Controller
            control={form.control}
            name="reauthorize_with_profiles"
            render={({ field }) => (
              <Checkbox
                checked={field.value ?? false}
                onCheckedChange={field.onChange}
                disabled={pending}
              />
            )}
          />
          使用已保存资料自动重新授权
        </label>
        <div className="grid gap-3 sm:grid-cols-2">
          <FormField
            label="检查间隔（分钟）"
            htmlFor="workbench-interval"
            error={form.formState.errors.interval_minutes?.message}
          >
            <Input
              id="workbench-interval"
              type="number"
              min={1}
              disabled={pending}
              aria-invalid={!!form.formState.errors.interval_minutes}
              {...form.register("interval_minutes", { valueAsNumber: true })}
            />
          </FormField>
          <FormField
            label="修复冷却时间（分钟）"
            htmlFor="workbench-cooldown"
            error={form.formState.errors.cooldown_minutes?.message}
          >
            <Input
              id="workbench-cooldown"
              type="number"
              min={1}
              disabled={pending}
              aria-invalid={!!form.formState.errors.cooldown_minutes}
              {...form.register("cooldown_minutes", { valueAsNumber: true })}
            />
          </FormField>
        </div>
        <Controller
          control={form.control}
          name="group_ids"
          render={({ field }) => (
            <WorkbenchGroupPicker
              groups={props.groups}
              value={field.value}
              onChange={field.onChange}
              disabled={pending}
            />
          )}
        />
        <label className="flex items-center gap-2 text-sm">
          <Controller
            control={form.control}
            name="check_after_repair"
            render={({ field }) => (
              <Checkbox checked={field.value} onCheckedChange={field.onChange} disabled={pending} />
            )}
          />
          修复后检测（会产生模型调用用量）
        </label>
        <FormField
          label="检测模型"
          htmlFor="workbench-maintenance-model"
          error={form.formState.errors.model?.message}
        >
          <Input
            id="workbench-maintenance-model"
            disabled={pending || !form.watch("check_after_repair")}
            aria-invalid={!!form.formState.errors.model}
            {...form.register("model")}
          />
        </FormField>
        <div className="flex flex-wrap gap-2">
          <Button type="submit" disabled={pending}>
            {save.isPending ? "正在保存…" : "保存维护设置"}
          </Button>
          {(form.formState.isDirty || baseline.revision !== props.config.revision) && (
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => {
                setBaseline(props.config);
                form.reset(props.config);
              }}
            >
              <RotateCcw aria-hidden="true" /> 重置修改
            </Button>
          )}
          <Button
            type="button"
            variant="outline"
            disabled={pending || form.formState.isDirty}
            onClick={() => setConfirm("check")}
          >
            立即检查并修复
          </Button>
        </div>
        {form.formState.isDirty && (
          <p className="text-sm text-muted-foreground">请先保存设置，再立即检查。</p>
        )}
        {check.isPending && <TaskStartupState message="正在创建账号维护任务" />}
        {props.config.last_run_at && (
          <p className="text-sm text-muted-foreground">最近检查：{props.config.last_run_at}</p>
        )}
      </form>
      <ConfirmActionDialog
        open={confirm !== null}
        title={confirm === "check" ? "确认立即检查并修复" : "确认启用自动维护"}
        description={`范围：${scope}。检查并修复符合条件的账号登录状态。${detection}${confirmedSettings.reauthorize_with_profiles ? "使用服务器已保存的登录资料自动重新授权，并更新原账号凭据；不会自动购买短信。退出登录或重启服务后需重新连接会话。" : ""}`}
        confirmLabel={confirm === "check" ? "创建维护任务" : "保存并启用"}
        pending={pending}
        onOpenChange={(open) => {
          if (!open) setConfirm(null);
        }}
        onConfirm={() => {
          if (confirm === "check") check.mutate();
          else if (confirm) save.mutate(confirm);
        }}
      />
      {task && <WorkbenchTask task={task} />}
      {props.config.enabled && props.config.reauthorize_with_profiles ? (
        <WorkbenchMaintenanceAuthorization config={props.config} />
      ) : null}
    </div>
  );
}
