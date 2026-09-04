import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Save } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { api, type AccountControlAction, type AccountDetail, type Task } from "@/api";
import { Button } from "@/components/ui/button";
import { FieldLabel } from "@/components/field-help-tooltip";
import { DialogBody, DialogFooter } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { accountPoolState } from "@/features/accounts/lib/account-pool";
import { notifyOperationError } from "@/lib/operation-feedback";
import { taskStopsPolling } from "@/lib/task-state";

import { accountDetailDialogLayout } from "./account-detail-dialog";

const positiveInteger = z
  .string()
  .trim()
  .regex(/^\d+$/, "请输入正整数")
  .refine((value) => Number(value) >= 1 && Number(value) <= 10_000_000, "请输入 1 到 10000000");

const settingsSchema = z.object({
  priority: positiveInteger,
  loadFactor: z
    .string()
    .trim()
    .refine(
      (value) => Number.isFinite(Number(value)) && Number(value) >= 1,
      "负载因子必须大于或等于 1",
    ),
  concurrency: positiveInteger,
  testModel: z.string().trim().max(256, "探测模型不能超过 256 个字符"),
  paused: z.boolean(),
  excluded: z.boolean(),
});

type AccountSettingsValues = z.infer<typeof settingsSchema>;

type AccountSettingControlState = Pick<AccountSettingsValues, "excluded" | "paused">;

const inheritedTestModelValue = "\u0000inherited-test-model";

export function accountTestModelOptions(models: string[], currentModel: string): string[] {
  const normalized = [...models, currentModel]
    .map((model) => model.trim())
    .filter((model) => model.length > 0);
  return [...new Set(normalized)].sort((left, right) => left.localeCompare(right));
}

export function accountSettingControlActions(
  previous: AccountSettingControlState,
  next: AccountSettingControlState,
): AccountControlAction[] {
  const actions: AccountControlAction[] = [];
  if (next.excluded !== previous.excluded) actions.push(next.excluded ? "exclude" : "include");
  if (next.paused !== previous.paused) actions.push(next.paused ? "pause" : "resume");
  return actions;
}

function waitForNextTaskPoll(): Promise<void> {
  return new Promise((resolve) => globalThis.setTimeout(resolve, 500));
}

export async function waitForAccountSettingTasks(
  initial: Task[],
  load: (taskID: string) => Promise<Task>,
  wait: () => Promise<void> = waitForNextTaskPoll,
): Promise<Task[]> {
  let current = initial;
  while (!current.every(taskStopsPolling)) {
    current = await Promise.all(
      current.map((task) => (taskStopsPolling(task) ? task : load(task.id))),
    );
    if (!current.every(taskStopsPolling)) await wait();
  }
  const unsuccessful = current.filter((task) => task.status !== "succeeded");
  if (unsuccessful.length > 0) {
    throw new Error(
      unsuccessful.map((task) => task.message || `任务 ${task.id} 未成功完成`).join("；"),
    );
  }
  return current;
}

export function AccountSettingsPanel(props: {
  accountId: string;
  query: {
    data?: AccountDetail;
    isLoading: boolean;
    isError: boolean;
    error: unknown;
  };
  onCancel: () => void;
  onSaved: () => void;
}) {
  const detail = props.query.data;
  const queryClient = useQueryClient();
  const [models, setModels] = useState<string[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [modelsLoaded, setModelsLoaded] = useState(false);
  const form = useForm<AccountSettingsValues>({
    resolver: zodResolver(settingsSchema),
    defaultValues: {
      priority: "",
      loadFactor: "",
      concurrency: "",
      testModel: "",
      paused: false,
      excluded: false,
    },
  });

  useEffect(() => {
    if (!detail) return;
    form.reset({
      priority: detail.priority == null ? "" : String(detail.priority),
      loadFactor: detail.load_factor ?? "",
      concurrency: detail.concurrency == null ? "" : String(detail.concurrency),
      testModel: detail.test_model ?? "",
      paused: detail.paused === true,
      excluded: accountPoolState(detail).value === "excluded",
    });
  }, [detail, form]);

  const save = useMutation({
    mutationFn: async (values: AccountSettingsValues) => {
      if (!detail) throw new Error("账号详情尚未读取完成");
      const tasks: Task[] = [];
      const wasExcluded = accountPoolState(detail).value === "excluded";
      const controls = accountSettingControlActions(
        { excluded: wasExcluded, paused: detail.paused === true },
        values,
      );
      for (const action of controls.slice(0, values.excluded !== wasExcluded ? 1 : 0)) {
        tasks.push(await api.setAccountControl(props.accountId, action));
      }
      const originalModel = detail.test_model ?? "";
      if (values.testModel !== originalModel) {
        await api.setAccountTestModel(props.accountId, values.testModel || null);
      }
      const fields: Parameters<typeof api.syncAccount>[1] = {};
      if (values.priority !== String(detail.priority ?? ""))
        fields.priority = Number(values.priority);
      if (values.loadFactor !== (detail.load_factor ?? "")) fields.load_factor = values.loadFactor;
      if (values.concurrency !== String(detail.concurrency ?? "")) {
        fields.concurrency = Number(values.concurrency);
      }
      if (Object.keys(fields).length > 0) {
        tasks.push(await api.syncAccount(props.accountId, fields));
      }
      for (const action of controls.slice(values.excluded !== wasExcluded ? 1 : 0)) {
        tasks.push(await api.setAccountControl(props.accountId, action));
      }
      return waitForAccountSettingTasks(tasks, api.task);
    },
    onSuccess: async (tasks) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["accounts"] }),
        queryClient.invalidateQueries({ queryKey: ["account-detail", props.accountId] }),
        queryClient.invalidateQueries({ queryKey: ["policy"] }),
        queryClient.invalidateQueries({ queryKey: ["logs"] }),
      ]);
      toast.success(tasks.length ? "账号设置已执行完成" : "账号设置已保存");
      props.onSaved();
    },
    onError: (error) => {
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ["accounts"] }),
        queryClient.invalidateQueries({ queryKey: ["account-detail", props.accountId] }),
        queryClient.invalidateQueries({ queryKey: ["policy"] }),
        queryClient.invalidateQueries({ queryKey: ["logs"] }),
      ]);
      notifyOperationError(error, "账号设置保存失败");
    },
  });

  async function loadModels() {
    setModelsLoading(true);
    try {
      const result = await api.accountModels(props.accountId);
      setModels(result.models);
      setModelsLoaded(true);
      toast.success(`已读取 ${result.models.length} 个模型`);
    } catch (error) {
      notifyOperationError(error, "账号模型读取失败");
    } finally {
      setModelsLoading(false);
    }
  }

  const formId = `account-settings-${props.accountId}`;
  const testModel = form.watch("testModel");
  const modelOptions = accountTestModelOptions(models, testModel);
  const fetchedModelCount = accountTestModelOptions(models, "").length;
  let modelsButtonLabel = "获取上游模型";
  if (modelsLoading) modelsButtonLabel = "获取中";
  else if (modelsLoaded) modelsButtonLabel = "重新获取上游模型";
  return (
    <>
      <DialogBody className={accountDetailDialogLayout.body}>
        {props.query.isLoading ? <AccountSettingsSkeleton /> : null}
        {props.query.isError ? (
          <p className="text-destructive py-6 text-center text-sm">
            {props.query.error instanceof Error ? props.query.error.message : "账号详情读取失败"}
          </p>
        ) : null}
        {detail ? (
          <form
            id={formId}
            className="grid gap-4"
            onSubmit={form.handleSubmit((values) => save.mutate(values))}
          >
            <section aria-labelledby={`${formId}-routing`} className="grid gap-3">
              <SettingsSectionHeading
                id={`${formId}-routing`}
                title="调度参数"
                description="调整该账号参与分组调度时使用的基础参数。"
              />
              <div
                className="grid gap-x-4 gap-y-3 sm:grid-cols-2"
                data-testid="account-routing-grid"
              >
                <SettingsField
                  label="优先级"
                  error={form.formState.errors.priority?.message}
                  hint="数值越小越优先"
                >
                  <Input type="number" min={1} {...form.register("priority")} />
                </SettingsField>
                <SettingsField label="负载因子" error={form.formState.errors.loadFactor?.message}>
                  <Input type="number" min={1} step="any" {...form.register("loadFactor")} />
                </SettingsField>
                <SettingsField label="并发上限" error={form.formState.errors.concurrency?.message}>
                  <Input type="number" min={1} {...form.register("concurrency")} />
                </SettingsField>
                <SettingsField
                  label="账号成本"
                  hint="由上游原始倍率除以充值比例自动计算；请使用“同步倍率”更新"
                >
                  <Input
                    type="number"
                    min="0.000001"
                    step="any"
                    readOnly
                    aria-readonly="true"
                    value={detail.multiplier ?? ""}
                  />
                </SettingsField>
              </div>
            </section>

            <section aria-labelledby={`${formId}-control`} className="grid gap-3 border-t pt-4">
              <SettingsSectionHeading
                id={`${formId}-control`}
                title="账号管控"
                description="控制账号是否参与调度、探测和健康评分。"
              />
              <div
                className="divide-border divide-y overflow-hidden rounded-lg border"
                data-testid="account-control-group"
              >
                <SettingsSwitch
                  id={`${formId}-paused`}
                  label="暂停调度"
                  description="停止接收流量，继续监控计分，不自动恢复。"
                  checked={form.watch("paused")}
                  onCheckedChange={(checked) =>
                    form.setValue("paused", checked, { shouldDirty: true })
                  }
                />
                <SettingsSwitch
                  id={`${formId}-excluded`}
                  label="排除该账号"
                  description="不探测、不调度、不计分，并恢复接管前配置。"
                  checked={form.watch("excluded")}
                  onCheckedChange={(checked) =>
                    form.setValue("excluded", checked, { shouldDirty: true })
                  }
                />
              </div>
            </section>

            <section aria-labelledby={`${formId}-model`} className="grid gap-3 border-t pt-4">
              <SettingsSectionHeading
                id={`${formId}-model`}
                title="探测模型"
                description="留空时继承分组或全局默认模型。"
              />
              <div className="grid min-w-0 gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
                {modelsLoaded && modelOptions.length > 0 ? (
                  <Select
                    value={testModel.trim() || inheritedTestModelValue}
                    itemToStringLabel={(value) =>
                      value === inheritedTestModelValue ? "继承分组或全局默认模型" : value
                    }
                    onValueChange={(value) => {
                      if (!value) return;
                      form.setValue("testModel", value === inheritedTestModelValue ? "" : value, {
                        shouldDirty: true,
                        shouldValidate: true,
                      });
                    }}
                  >
                    <SelectTrigger
                      className="min-w-0"
                      aria-label="选择探测模型"
                      aria-invalid={Boolean(form.formState.errors.testModel)}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent align="start">
                      <SelectItem value={inheritedTestModelValue}>
                        继承分组或全局默认模型
                      </SelectItem>
                      {modelOptions.map((model) => (
                        <SelectItem key={model} value={model}>
                          {model}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <Input
                    className="min-w-0"
                    aria-label="探测模型"
                    aria-invalid={Boolean(form.formState.errors.testModel)}
                    placeholder="留空使用分组或全局默认模型"
                    {...form.register("testModel")}
                  />
                )}
                <Button
                  type="button"
                  variant="outline"
                  className="whitespace-nowrap"
                  disabled={modelsLoading}
                  onClick={() => void loadModels()}
                >
                  <RefreshCw className={modelsLoading ? "animate-spin" : undefined} />
                  {modelsButtonLabel}
                </Button>
              </div>
              {form.formState.errors.testModel?.message ? (
                <span className="text-destructive text-xs">
                  {form.formState.errors.testModel.message}
                </span>
              ) : null}
              {modelsLoaded ? (
                <p className="text-muted-foreground text-xs" role="status">
                  {fetchedModelCount > 0
                    ? `已读取 ${fetchedModelCount} 个上游模型`
                    : "上游未返回可用模型，可继续手动输入"}
                </p>
              ) : null}
            </section>
          </form>
        ) : null}
      </DialogBody>
      <DialogFooter>
        <Button type="button" variant="outline" disabled={save.isPending} onClick={props.onCancel}>
          取消
        </Button>
        <Button type="submit" form={formId} disabled={!detail || save.isPending}>
          <Save />
          {save.isPending ? "保存中…" : "保存"}
        </Button>
      </DialogFooter>
    </>
  );
}

function SettingsField(props: {
  label: string;
  error?: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="grid min-w-0 gap-1.5 text-sm">
      <FieldLabel label={props.label} description={!props.error ? props.hint : undefined} />
      {props.children}
      {props.error ? <span className="text-destructive text-xs">{props.error}</span> : null}
    </div>
  );
}

function SettingsSwitch(props: {
  id: string;
  label: string;
  description: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="hover:bg-muted/35 flex min-h-16 items-center justify-between gap-4 px-3 py-3 transition-colors">
      <label className="grid min-w-0 cursor-pointer gap-1" htmlFor={props.id}>
        <span className="font-medium">{props.label}</span>
        <span className="text-muted-foreground text-xs leading-4">{props.description}</span>
      </label>
      <Switch
        id={props.id}
        checked={props.checked}
        aria-label={props.label}
        className="shrink-0"
        onCheckedChange={props.onCheckedChange}
      />
    </div>
  );
}

function SettingsSectionHeading(props: { id: string; title: string; description: string }) {
  return (
    <div className="grid gap-1">
      <h3 id={props.id} className="text-sm font-medium">
        {props.title}
      </h3>
      <p className="text-muted-foreground text-xs leading-4">{props.description}</p>
    </div>
  );
}

function AccountSettingsSkeleton() {
  return (
    <div className="grid gap-4" aria-label="正在读取账号设置">
      <div className="grid gap-3 sm:grid-cols-2">
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
      </div>
      <Skeleton className="h-32" />
      <Skeleton className="h-16" />
    </div>
  );
}
