import { useEffect, useId } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing, Plus, RotateCcw, Save, Settings2, ShieldAlert, Trash2 } from "lucide-react";
import { Controller, useFieldArray, useForm } from "react-hook-form";
import { toast } from "sonner";

import { api, type AlertPolicy } from "@/api";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  alertPolicyFormSchema,
  defaultAlertPolicyForm,
  type AlertPolicyFormValues,
} from "../lib/alert-policy-schema";

type AlertPolicyPageProps = {
  onOpenSettings: () => void;
};

const ruleFields: Array<{
  name:
    | "configuration_enabled"
    | "auth_enabled"
    | "rate_sync_enabled"
    | "balance_enabled"
    | "probe_enabled"
    | "routing_breaker_enabled"
    | "routing_degraded_enabled"
    | "routing_survivor_enabled"
    | "group_unavailable_enabled"
    | "group_survivor_enabled"
    | "apply_failure_enabled";
  label: string;
  description: string;
}> = [
  {
    name: "configuration_enabled",
    label: "配置异常",
    description: "上游状态、余额或元数据无法解析时产生告警",
  },
  { name: "auth_enabled", label: "鉴权失效", description: "Token 失效、过期、未鉴权或恢复失败" },
  { name: "rate_sync_enabled", label: "倍率同步失败", description: "最近一次上游倍率同步任务失败" },
  { name: "balance_enabled", label: "余额不足", description: "余额达到阈值或上游已触发余额硬关闭" },
  {
    name: "probe_enabled",
    label: "主动探测失败",
    description: "账号在分组中的连续主动探测结果未通过",
  },
  {
    name: "routing_breaker_enabled",
    label: "账号熔断判定",
    description: "调度策略判定账号需要停止接收流量",
  },
  {
    name: "routing_degraded_enabled",
    label: "账号降级",
    description: "健康分或响应质量达到调度策略的降级条件",
  },
  {
    name: "routing_survivor_enabled",
    label: "保底强留",
    description: "账号本应熔断，但为避免分组断供而继续保留",
  },
  {
    name: "group_unavailable_enabled",
    label: "分组无可调度账号",
    description: "本轮调度判定后，分组内没有账号可以接收流量",
  },
  {
    name: "group_survivor_enabled",
    label: "分组仅剩保底账号",
    description: "本轮调度判定后，分组仅靠保底账号维持服务",
  },
  {
    name: "apply_failure_enabled",
    label: "自动执行失败",
    description: "调度目标写入远端失败，或写入后无法确认实际状态",
  },
];

const notificationTargetTypeLabels: Record<string, string> = {
  c2c: "私聊",
  group: "群聊",
  channel: "频道",
};

function notificationTargetTypeLabel(channelType: string): string {
  return notificationTargetTypeLabels[channelType] ?? "未知目标";
}

function policyToForm(policy: AlertPolicy): AlertPolicyFormValues {
  return {
    ...policy,
    balance_thresholds: policy.balance_thresholds.map((value) => ({ value })),
    probe_groups: policy.probe_groups.join(", "),
  };
}

function splitGroups(value: string): string[] {
  return [
    ...new Set(
      value
        .split(/[,，\n]/)
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  ];
}

function SettingSwitch(props: {
  label: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const switchId = useId();
  return (
    <div className="flex min-h-16 items-center justify-between gap-4 border-b py-3 last:border-b-0">
      <label
        className={props.disabled ? "min-w-0 cursor-not-allowed" : "min-w-0 cursor-pointer"}
        htmlFor={switchId}
      >
        <p className="text-sm font-medium">{props.label}</p>
        <p className="text-muted-foreground mt-1 text-xs leading-5">{props.description}</p>
      </label>
      <Switch
        id={switchId}
        checked={props.checked}
        disabled={props.disabled}
        onCheckedChange={props.onCheckedChange}
        aria-label={props.label}
      />
    </div>
  );
}

function LoadingPolicy() {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {[0, 1, 2, 3].map((item) => (
        <Card key={item} className="min-h-52 p-4">
          <Skeleton className="h-6 w-28" />
          <Skeleton className="mt-5 h-10 w-full" />
          <Skeleton className="mt-3 h-10 w-full" />
          <Skeleton className="mt-3 h-10 w-3/4" />
        </Card>
      ))}
    </div>
  );
}

export function AlertPolicyPage(props: AlertPolicyPageProps) {
  const queryClient = useQueryClient();
  const policy = useQuery({ queryKey: ["alert-policy"], queryFn: api.alertPolicy });
  const notification = useQuery({
    queryKey: ["notification-status"],
    queryFn: api.notificationStatus,
  });
  const form = useForm<AlertPolicyFormValues>({
    resolver: zodResolver(alertPolicyFormSchema),
    defaultValues: defaultAlertPolicyForm,
  });
  const balanceThresholds = useFieldArray({ control: form.control, name: "balance_thresholds" });
  const update = useMutation({
    mutationFn: api.updateAlertPolicy,
    onSuccess: (saved) => {
      queryClient.setQueryData(["alert-policy"], saved);
      form.reset(policyToForm(saved));
      toast.success("告警策略已保存");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "告警策略保存失败"),
  });

  useEffect(() => {
    if (policy.data) form.reset(policyToForm(policy.data));
  }, [form, policy.data]);

  const enabled = form.watch("enabled");
  const balanceEnabled = form.watch("balance_enabled");
  const probeEnabled = form.watch("probe_enabled");
  const deliveryEnabled = form.watch("delivery_enabled");

  const submit = form.handleSubmit((values) => {
    update.mutate({
      ...values,
      balance_thresholds: values.balance_thresholds.map((item) => item.value.trim()),
      probe_groups: splitGroups(values.probe_groups),
    });
  });

  return (
    <PageLayout>
      <PageHeading
        eyebrow="OPERATIONS / ALERT POLICY"
        title="告警策略"
        description="配置告警检测范围、触发阈值和通知发送行为；渠道凭据统一在系统设置中管理。"
        action={
          <PageActions>
            <Button
              variant="outline"
              onClick={() => form.reset(defaultAlertPolicyForm)}
              disabled={update.isPending}
            >
              <RotateCcw /> 恢复默认
            </Button>
            <Button onClick={() => void submit()} disabled={update.isPending || policy.isLoading}>
              <Save /> {update.isPending ? "保存中" : "保存策略"}
            </Button>
          </PageActions>
        }
      />

      {policy.error && <QueryErrorToast error={policy.error} fallback="告警策略读取失败" />}
      {policy.isLoading ? (
        <LoadingPolicy />
      ) : (
        <form
          onSubmit={submit}
          data-slot="alert-policy-columns"
          className="grid items-start gap-4 lg:grid-cols-2"
        >
          <div className="grid min-w-0 gap-4">
            <Card>
              <CardHeader className="border-b">
                <CardTitle className="flex items-center gap-2">
                  <ShieldAlert className="text-primary" />
                  告警检测
                </CardTitle>
              </CardHeader>
              <CardContent>
                <Controller
                  control={form.control}
                  name="enabled"
                  render={({ field }) => (
                    <SettingSwitch
                      label="启用告警检测"
                      description="关闭后保留现有告警记录，不再检查新的异常或恢复状态"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b">
                <CardTitle>阈值与范围</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-5 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <div className="flex items-center justify-between gap-3">
                    <span className="text-sm font-medium">余额告警阈值</span>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={
                        !enabled || !balanceEnabled || balanceThresholds.fields.length >= 20
                      }
                      onClick={() => balanceThresholds.append({ value: "" })}
                    >
                      <Plus /> 添加阈值
                    </Button>
                  </div>
                  <div
                    className="mt-3 flex flex-wrap items-start gap-2"
                    data-slot="balance-threshold-list"
                  >
                    {balanceThresholds.fields.map((field, index) => (
                      <div className="w-36 max-w-full" key={field.id}>
                        <div className="relative">
                          <Input
                            id={`balance-threshold-${index}`}
                            className="pr-9"
                            aria-label={`余额告警阈值 ${index + 1}`}
                            aria-invalid={Boolean(
                              form.formState.errors.balance_thresholds?.[index]?.value,
                            )}
                            inputMode="decimal"
                            placeholder="输入阈值"
                            disabled={!enabled || !balanceEnabled}
                            {...form.register(`balance_thresholds.${index}.value`)}
                          />
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            className="absolute inset-y-0 right-1 z-10 my-auto size-7"
                            aria-label={`删除余额告警阈值 ${index + 1}`}
                            disabled={
                              !enabled || !balanceEnabled || balanceThresholds.fields.length === 1
                            }
                            onClick={() => balanceThresholds.remove(index)}
                          >
                            <Trash2 size={15} />
                          </Button>
                        </div>
                        {form.formState.errors.balance_thresholds?.[index]?.value && (
                          <p className="text-destructive mt-1 text-xs">
                            {form.formState.errors.balance_thresholds[index]?.value?.message}
                          </p>
                        )}
                      </div>
                    ))}
                  </div>
                  <p className="text-muted-foreground mt-1.5 text-xs">
                    支持多个提醒档位，例如 20、10、5；余额达到或低于下一档时会再次告警。
                  </p>
                  {form.formState.errors.balance_thresholds?.root && (
                    <p className="text-destructive mt-1 text-xs">
                      {form.formState.errors.balance_thresholds.root.message}
                    </p>
                  )}
                </div>
                <div>
                  <label className="text-sm font-medium" htmlFor="probe_failure_streak">
                    连续主动探测失败次数
                  </label>
                  <Input
                    id="probe_failure_streak"
                    type="number"
                    min={1}
                    max={100}
                    className="mt-2"
                    disabled={!enabled || !probeEnabled}
                    {...form.register("probe_failure_streak", { valueAsNumber: true })}
                  />
                  <p className="text-muted-foreground mt-1.5 text-xs">
                    达到次数后才产生主动探测告警。
                  </p>
                  {form.formState.errors.probe_failure_streak && (
                    <p className="text-destructive mt-1 text-xs">
                      {form.formState.errors.probe_failure_streak.message}
                    </p>
                  )}
                </div>
                <div>
                  <label className="text-sm font-medium" htmlFor="probe_recovery_streak">
                    连续主动探测成功次数
                  </label>
                  <Input
                    id="probe_recovery_streak"
                    type="number"
                    min={1}
                    max={100}
                    className="mt-2"
                    disabled={!enabled || !probeEnabled}
                    {...form.register("probe_recovery_streak", { valueAsNumber: true })}
                  />
                  <p className="text-muted-foreground mt-1.5 text-xs">
                    达到次数后才确认恢复并发送恢复通知。
                  </p>
                  {form.formState.errors.probe_recovery_streak && (
                    <p className="text-destructive mt-1 text-xs">
                      {form.formState.errors.probe_recovery_streak.message}
                    </p>
                  )}
                </div>
                <div className="sm:col-span-2">
                  <label className="text-sm font-medium" htmlFor="probe_groups">
                    主动探测告警分组
                  </label>
                  <Input
                    id="probe_groups"
                    className="mt-2"
                    placeholder="留空表示全部分组，多个分组用逗号分隔"
                    disabled={!enabled || !probeEnabled}
                    {...form.register("probe_groups")}
                  />
                  <p className="text-muted-foreground mt-1.5 text-xs">
                    仅限制主动探测失败规则，其他上游告警不受影响。
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b">
                <CardTitle className="flex items-center justify-between gap-3">
                  <span className="flex items-center gap-2">
                    <BellRing className="text-primary" />
                    通知渠道
                  </span>
                  <Badge variant={notification.data?.configured ? "secondary" : "warning"}>
                    {notification.data?.configured ? "已连接" : "未配置"}
                  </Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
                <div className="min-w-0">
                  <p className="text-sm">当前支持 QQBot 通知渠道。</p>
                  <p className="text-muted-foreground mt-1 text-xs leading-5">
                    {notification.data?.configured
                      ? `目标类型：${notificationTargetTypeLabel(notification.data.channel_type)}。敏感凭据继续由系统设置统一管理。`
                      : "未配置渠道时仍会检测并保存告警，但不会发送通知。"}
                  </p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  className="shrink-0"
                  onClick={props.onOpenSettings}
                >
                  <Settings2 /> 管理通知渠道
                </Button>
              </CardContent>
            </Card>
          </div>

          <div className="grid min-w-0 gap-4">
            <Card>
              <CardHeader className="border-b">
                <CardTitle className="flex items-center gap-2">
                  <BellRing className="text-primary" />
                  通知发送
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid gap-x-6 sm:grid-cols-2" data-slot="alert-delivery-switches">
                  <Controller
                    control={form.control}
                    name="delivery_enabled"
                    render={({ field }) => (
                      <SettingSwitch
                        label="启用通知发送"
                        description="不影响异常检测和告警记录，仅控制是否发送通知消息"
                        checked={field.value}
                        disabled={!enabled}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                  <Controller
                    control={form.control}
                    name="notify_recovery"
                    render={({ field }) => (
                      <SettingSwitch
                        label="发送恢复通知"
                        description="异常恢复时发送一次恢复消息"
                        checked={field.value}
                        disabled={!enabled || !deliveryEnabled}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                </div>
                <div
                  className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-3"
                  data-slot="alert-delivery-fields"
                >
                  <div>
                    <label className="block text-sm font-medium" htmlFor="repeat_interval_minutes">
                      重复提醒间隔（分钟）
                    </label>
                    <Input
                      id="repeat_interval_minutes"
                      type="number"
                      min={0}
                      max={10080}
                      className="mt-2"
                      disabled={!enabled || !deliveryEnabled}
                      {...form.register("repeat_interval_minutes", { valueAsNumber: true })}
                    />
                    <p className="text-muted-foreground mt-1.5 text-xs">
                      设为 0 表示持续告警只发送一次。
                    </p>
                    {form.formState.errors.repeat_interval_minutes && (
                      <p className="text-destructive mt-1 text-xs">
                        {form.formState.errors.repeat_interval_minutes.message}
                      </p>
                    )}
                  </div>
                  <div>
                    <label
                      className="block text-sm font-medium"
                      htmlFor="state_change_cooldown_minutes"
                    >
                      状态变化冷却（分钟）
                    </label>
                    <Input
                      id="state_change_cooldown_minutes"
                      type="number"
                      min={0}
                      max={10080}
                      className="mt-2"
                      disabled={!enabled || !deliveryEnabled}
                      {...form.register("state_change_cooldown_minutes", { valueAsNumber: true })}
                    />
                    <p className="text-muted-foreground mt-1.5 text-xs">
                      异常与恢复反复切换时，只在冷却结束后发送当前状态。
                    </p>
                    {form.formState.errors.state_change_cooldown_minutes && (
                      <p className="text-destructive mt-1 text-xs">
                        {form.formState.errors.state_change_cooldown_minutes.message}
                      </p>
                    )}
                  </div>
                  <div>
                    <label className="block text-sm font-medium" htmlFor="merge_threshold">
                      多少条以上合并发送
                    </label>
                    <Input
                      id="merge_threshold"
                      type="number"
                      min={2}
                      max={500}
                      className="mt-2"
                      disabled={!enabled || !deliveryEnabled}
                      {...form.register("merge_threshold", { valueAsNumber: true })}
                    />
                    <p className="text-muted-foreground mt-1.5 text-xs">
                      少于该数量时，每条告警单独发送。
                    </p>
                    {form.formState.errors.merge_threshold && (
                      <p className="text-destructive mt-1 text-xs">
                        {form.formState.errors.merge_threshold.message}
                      </p>
                    )}
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b">
                <CardTitle>检测规则</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-x-6 xl:grid-cols-2" data-slot="alert-rule-grid">
                {ruleFields.map((rule) => (
                  <Controller
                    key={rule.name}
                    control={form.control}
                    name={rule.name}
                    render={({ field }) => (
                      <SettingSwitch
                        label={rule.label}
                        description={rule.description}
                        checked={field.value}
                        disabled={!enabled}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                ))}
              </CardContent>
            </Card>
          </div>
        </form>
      )}
    </PageLayout>
  );
}
