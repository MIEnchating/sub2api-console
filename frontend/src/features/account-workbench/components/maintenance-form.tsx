import { useMemo, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { api } from "@/api";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { MultiSelect } from "@/components/multi-select";
import { ContentLoading } from "@/components/content-loading";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { maintenanceKey, workbenchKeys } from "../constants";
import { maintenanceSchema, type MaintenanceValues } from "../lib/maintenance-schema";
import type { MaintenancePreview, WorkbenchMaintenance } from "../types";

export function MaintenanceForm(props: { value: WorkbenchMaintenance }): ReactElement {
  const client = useQueryClient();
  const accounts = useQuery({ queryKey: workbenchKeys.accounts, queryFn: api.workbenchAccounts });
  const form = useForm<MaintenanceValues>({
    resolver: zodResolver(maintenanceSchema),
    defaultValues: props.value,
  });
  const [preview, setPreview] = useState<MaintenancePreview | null>(null);
  const groups = useMemo(() => {
    const result = new Map<string, string>();
    for (const account of accounts.data ?? [])
      for (const group of account.groups) result.set(group.id, group.name || `分组 #${group.id}`);
    return [...result].map(([value, label]) => ({ value, label }));
  }, [accounts.data]);
  const update = (value: WorkbenchMaintenance): void => {
    client.setQueryData(maintenanceKey, value);
    form.reset(value);
    void client.invalidateQueries({ queryKey: workbenchKeys.accounts });
  };
  const parse = useMutation({
    mutationFn: api.previewWorkbenchMaintenance,
    onSuccess: setPreview,
    onError: (error) => notifyOperationError(error, "维护范围读取失败"),
  });
  const save = useMutation({
    mutationFn: api.configureWorkbenchMaintenance,
    onSuccess: (value) => {
      update(value);
      setPreview(null);
    },
    onError: (error) => notifyOperationError(error, "维护设置保存失败"),
  });
  const action = useMutation({
    mutationFn: (kind: "check" | "stop") =>
      api.workbenchMaintenanceAction(kind, props.value.revision),
    onSuccess: update,
    onError: (error) => notifyOperationError(error, "维护操作失败"),
  });
  const busy = parse.isPending || save.isPending || action.isPending;
  return (
    <>
      <form
        onSubmit={form.handleSubmit((value) => parse.mutate(value))}
        aria-label="维护设置"
        className="grid min-w-0 gap-4 rounded-xl border bg-card p-4"
      >
        <div className="grid gap-1 border-b pb-3">
          <h3 className="text-sm font-semibold">维护设置</h3>
          <p className="text-xs leading-5 text-muted-foreground">
            设置检查范围与频率，保存前确认受影响账号。
          </p>
        </div>
        <fieldset disabled={busy || props.value.running} className="grid min-w-0 gap-4">
          <legend className="sr-only">维护设置</legend>
          <div className="grid gap-2">
            <label htmlFor="maintenance-groups" className="text-sm">
              维护分组
            </label>
            <Controller
              name="group_ids"
              control={form.control}
              render={({ field }) => (
                <MultiSelect
                  id="maintenance-groups"
                  ariaLabel="维护分组"
                  title="全部分组"
                  options={groups}
                  selected={field.value}
                  onChange={field.onChange}
                  disabled={busy || props.value.running || accounts.isPending}
                />
              )}
            />
            <p className="text-xs text-muted-foreground">
              不选分组时检查全部 OpenAI OAuth 账号；已手动停用的账号会跳过。
            </p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {(
              [
                { name: "interval_minutes", label: "检查间隔（分钟）", min: 1 },
                { name: "cooldown_minutes", label: "失败冷却（分钟）", min: 0 },
              ] as const
            ).map((item) => (
              <div key={item.name} className="grid gap-2">
                <label htmlFor={`maintenance-${item.name}`} className="text-sm">
                  {item.label}
                </label>
                <Input
                  id={`maintenance-${item.name}`}
                  type="number"
                  min={item.min}
                  max={1440}
                  aria-invalid={!!form.formState.errors[item.name]}
                  {...form.register(item.name, { valueAsNumber: true })}
                />
                {form.formState.errors[item.name] && (
                  <p role="alert" className="text-sm text-destructive">
                    {form.formState.errors[item.name]?.message}
                  </p>
                )}
              </div>
            ))}
          </div>
          <div className="grid gap-3 rounded-lg border bg-muted/20 p-3">
            {(
              [
                { name: "enabled", label: "启用定时检查" },
                { name: "check_after_repair", label: "恢复后执行 Sol 检测" },
              ] as const
            ).map((item) => (
              <label key={item.name} className="flex items-center gap-2 text-sm">
                <Controller
                  name={item.name}
                  control={form.control}
                  render={({ field }) => (
                    <Checkbox
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={busy || props.value.running}
                    />
                  )}
                />
                {item.label}
              </label>
            ))}
          </div>
        </fieldset>
        <div className="flex flex-wrap items-center gap-2 border-t pt-3">
          {parse.isPending && <ContentLoading compact label="正在读取维护范围" />}
          <Button
            type="button"
            variant="outline"
            disabled={busy || (!props.value.enabled && !props.value.running)}
            onClick={() => action.mutate("stop")}
          >
            停止维护
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={
              busy || props.value.running || !props.value.revision || form.formState.isDirty
            }
            onClick={() => action.mutate("check")}
          >
            立即检查
          </Button>
          <Button className="ml-auto" type="submit" disabled={busy || props.value.running}>
            预览并保存
          </Button>
        </div>
      </form>
      {preview && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !save.isPending) setPreview(null);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>确认维护范围 · {preview.accounts.length} 个账号</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <p className="text-sm">
                {preview.settings.enabled
                  ? `每 ${preview.settings.interval_minutes} 分钟检查所选分组，符合条件的新账号也会纳入检查。`
                  : "保存维护设置，定时检查保持关闭。"}{" "}
                维护可能刷新凭据、重新授权并恢复账号调度。
              </p>
              <div className="mt-3 max-h-56 overflow-y-auto rounded-md border p-3 text-sm">
                {preview.accounts.length === 0 ? (
                  "当前范围内暂无可维护账号"
                ) : (
                  <ul className="grid gap-2">
                    {preview.accounts.map((account) => (
                      <li key={account.id} className="break-all">
                        {account.email || account.name} · #{account.id}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </DialogBody>
            <DialogFooter>
              <Button variant="outline" disabled={save.isPending} onClick={() => setPreview(null)}>
                返回修改
              </Button>
              <Button
                disabled={save.isPending}
                onClick={() => save.mutate({ id: preview.id, revision: preview.revision })}
              >
                确认保存
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
