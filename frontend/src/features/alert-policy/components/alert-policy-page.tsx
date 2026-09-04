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
import { RefreshButton } from "@/components/refresh-button";
import { FieldLabel } from "@/components/field-help-tooltip";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  alertRuleFields,
  notificationTargetTypeLabels,
  recoveryNotificationFields,
  routingDegradedFields,
} from "../constants";
import {
  alertPolicyFormSchema,
  defaultAlertPolicyForm,
  type AlertPolicyFormValues,
} from "../lib/alert-policy-schema";

type AlertPolicyPageProps = {
  onOpenSettings: () => void;
};

function updateSelection<Value extends string>(
  values: Value[],
  value: Value,
  checked: boolean,
): Value[] {
  if (checked) return values.includes(value) ? values : [...values, value];
  return values.filter((item) => item !== value);
}

function notificationTargetTypeLabel(channelType: string): string {
  return notificationTargetTypeLabels[channelType] ?? "未知目标";
}

function policyToForm(policy: AlertPolicy): AlertPolicyFormValues {
  return {
    ...policy,
    routing_degraded_types:
      policy.routing_degraded_types ?? defaultAlertPolicyForm.routing_degraded_types,
    recovery_notification_types:
      policy.recovery_notification_types ?? defaultAlertPolicyForm.recovery_notification_types,
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

function PolicyUnavailable(props: { isFetching: boolean; onRetry: () => void }) {
  return (
    <Card data-testid="alert-policy-load-error" role="alert">
      <CardContent className="grid min-h-52 place-items-center p-6 text-center">
        <div>
          <ShieldAlert className="text-destructive mx-auto mb-3 size-6" />
          <p className="text-sm font-medium">告警策略暂不可用</p>
          <p className="text-muted-foreground mt-1 max-w-lg text-sm leading-6">
            未能读取现有策略。为避免覆盖当前配置，读取成功前无法编辑或保存。
          </p>
          <RefreshButton
            pending={props.isFetching}
            ariaLabel="刷新告警策略"
            onClick={props.onRetry}
            className="mt-4"
          />
        </div>
      </CardContent>
    </Card>
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
  const routingDegradedEnabled = form.watch("routing_degraded_enabled");
  const notifyRecovery = form.watch("notify_recovery");
  const policyReady = policy.data !== undefined && !policy.error;

  const submit = form.handleSubmit((values) => {
    if (!policyReady) {
      toast.error("告警策略尚未成功读取，请重试后再保存");
      return;
    }
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
              data-testid="alert-policy-reset"
              variant="outline"
              onClick={() => form.reset(defaultAlertPolicyForm)}
              disabled={update.isPending || !policyReady}
            >
              <RotateCcw /> 恢复默认
            </Button>
            <Button
              data-testid="alert-policy-save"
              onClick={() => void submit()}
              disabled={update.isPending || !policyReady}
            >
              <Save /> {update.isPending ? "保存中" : "保存策略"}
            </Button>
          </PageActions>
        }
      />

      {policy.error && <QueryErrorToast error={policy.error} fallback="告警策略读取失败" />}
      {policy.isLoading ? (
        <LoadingPolicy />
      ) : !policyReady ? (
        <PolicyUnavailable isFetching={policy.isFetching} onRetry={() => void policy.refetch()} />
      ) : (
        <form
          onSubmit={submit}
          data-slot="alert-policy-columns"
          className="grid items-start gap-4 lg:grid-cols-2"
        >
          <div
            className="order-2 grid min-w-0 gap-4 lg:col-start-2 lg:row-start-1"
            data-slot="alert-policy-threshold-column"
          >
            <Card>
              <CardHeader className="border-b">
                <CardTitle>阈值与范围</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-5 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <div className="flex items-center justify-between gap-3">
                    <FieldLabel
                      label="余额告警阈值"
                      description="支持多个提醒档位，例如 20、10、5；余额达到或低于下一档时会再次告警。"
                    />
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
                  {form.formState.errors.balance_thresholds?.root && (
                    <p className="text-destructive mt-1 text-xs">
                      {form.formState.errors.balance_thresholds.root.message}
                    </p>
                  )}
                </div>
                <div>
                  <FieldLabel
                    label="连续主动探测失败次数"
                    description="达到次数后才产生主动探测告警。"
                    htmlFor="probe_failure_streak"
                  />
                  <Input
                    id="probe_failure_streak"
                    type="number"
                    min={1}
                    max={100}
                    className="mt-2"
                    disabled={!enabled || !probeEnabled}
                    {...form.register("probe_failure_streak", { valueAsNumber: true })}
                  />
                  {form.formState.errors.probe_failure_streak && (
                    <p className="text-destructive mt-1 text-xs">
                      {form.formState.errors.probe_failure_streak.message}
                    </p>
                  )}
                </div>
                <div>
                  <FieldLabel
                    label="连续主动探测成功次数"
                    description="达到次数后才确认恢复并发送恢复通知。"
                    htmlFor="probe_recovery_streak"
                  />
                  <Input
                    id="probe_recovery_streak"
                    type="number"
                    min={1}
                    max={100}
                    className="mt-2"
                    disabled={!enabled || !probeEnabled}
                    {...form.register("probe_recovery_streak", { valueAsNumber: true })}
                  />
                  {form.formState.errors.probe_recovery_streak && (
                    <p className="text-destructive mt-1 text-xs">
                      {form.formState.errors.probe_recovery_streak.message}
                    </p>
                  )}
                </div>
                <div className="sm:col-span-2">
                  <FieldLabel
                    label="主动探测告警分组"
                    description="仅限制主动探测失败规则，其他上游告警不受影响。"
                    htmlFor="probe_groups"
                  />
                  <Input
                    id="probe_groups"
                    className="mt-2"
                    placeholder="留空表示全部分组，多个分组用逗号分隔"
                    disabled={!enabled || !probeEnabled}
                    {...form.register("probe_groups")}
                  />
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="border-b">
                <CardTitle>检测规则</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid gap-x-6 xl:grid-cols-2" data-slot="alert-rule-grid">
                  {alertRuleFields.map((rule) => (
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
                </div>
                <div className="mt-4 border-t pt-4" data-slot="routing-degraded-rules">
                  <Controller
                    control={form.control}
                    name="routing_degraded_enabled"
                    render={({ field }) => (
                      <SettingSwitch
                        label="账号降级"
                        description="控制全部账号降级告警；可继续选择需要关注的降级来源"
                        checked={field.value}
                        disabled={!enabled}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                  <div className="grid gap-x-6 pl-4 xl:grid-cols-2">
                    {routingDegradedFields.map((item) => (
                      <Controller
                        key={item.value}
                        control={form.control}
                        name="routing_degraded_types"
                        render={({ field }) => (
                          <SettingSwitch
                            label={item.label}
                            description={item.description}
                            checked={field.value.includes(item.value)}
                            disabled={!enabled || !routingDegradedEnabled}
                            onCheckedChange={(checked) =>
                              field.onChange(updateSelection(field.value, item.value, checked))
                            }
                          />
                        )}
                      />
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          <div
            className="order-1 grid min-w-0 gap-4 lg:col-start-1 lg:row-start-1"
            data-slot="alert-policy-detection-column"
          >
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
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <BellRing className="text-primary" />
                  通知发送
                </CardTitle>
                <CardAction
                  className="flex items-center gap-1.5"
                  data-slot="notification-channel-summary"
                >
                  <Badge variant={notification.data?.configured ? "secondary" : "warning"}>
                    {notification.data?.configured
                      ? `QQBot · ${notificationTargetTypeLabel(notification.data.channel_type)}`
                      : "QQBot 未配置"}
                  </Badge>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          aria-label="管理通知渠道"
                          onClick={props.onOpenSettings}
                        />
                      }
                    >
                      <Settings2 />
                    </TooltipTrigger>
                    <TooltipContent>管理通知渠道</TooltipContent>
                  </Tooltip>
                </CardAction>
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
                        description="仅为下方选中的告警类型发送一次恢复消息"
                        checked={field.value}
                        disabled={!enabled || !deliveryEnabled}
                        onCheckedChange={field.onChange}
                      />
                    )}
                  />
                </div>
                <div className="mt-4 border-t pt-4" data-slot="recovery-notification-types">
                  <p className="text-sm font-medium">恢复通知类型</p>
                  <p className="text-muted-foreground mt-1 text-xs leading-5">
                    默认关闭容易频繁波动或无需闭环确认的恢复消息，告警记录仍会正常更新。
                  </p>
                  <div className="mt-2 grid gap-x-6 sm:grid-cols-2">
                    {recoveryNotificationFields.map((item) => (
                      <Controller
                        key={item.value}
                        control={form.control}
                        name="recovery_notification_types"
                        render={({ field }) => (
                          <SettingSwitch
                            label={item.label}
                            description={item.description}
                            checked={field.value.includes(item.value)}
                            disabled={!enabled || !deliveryEnabled || !notifyRecovery}
                            onCheckedChange={(checked) =>
                              field.onChange(updateSelection(field.value, item.value, checked))
                            }
                          />
                        )}
                      />
                    ))}
                  </div>
                </div>
                <div
                  className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-3"
                  data-slot="alert-delivery-fields"
                >
                  <div>
                    <FieldLabel
                      label="重复提醒间隔（分钟）"
                      description="设为 0 表示持续告警只发送一次。"
                      htmlFor="repeat_interval_minutes"
                    />
                    <Input
                      id="repeat_interval_minutes"
                      type="number"
                      min={0}
                      max={10080}
                      className="mt-2"
                      disabled={!enabled || !deliveryEnabled}
                      {...form.register("repeat_interval_minutes", { valueAsNumber: true })}
                    />
                    {form.formState.errors.repeat_interval_minutes && (
                      <p className="text-destructive mt-1 text-xs">
                        {form.formState.errors.repeat_interval_minutes.message}
                      </p>
                    )}
                  </div>
                  <div>
                    <FieldLabel
                      label="状态变化冷却（分钟）"
                      description="异常与恢复反复切换时，只在冷却结束后发送当前状态。"
                      htmlFor="state_change_cooldown_minutes"
                    />
                    <Input
                      id="state_change_cooldown_minutes"
                      type="number"
                      min={0}
                      max={10080}
                      className="mt-2"
                      disabled={!enabled || !deliveryEnabled}
                      {...form.register("state_change_cooldown_minutes", { valueAsNumber: true })}
                    />
                    {form.formState.errors.state_change_cooldown_minutes && (
                      <p className="text-destructive mt-1 text-xs">
                        {form.formState.errors.state_change_cooldown_minutes.message}
                      </p>
                    )}
                  </div>
                  <div>
                    <FieldLabel
                      label="多少条以上合并发送"
                      description="少于该数量时，每条告警单独发送。"
                      htmlFor="merge_threshold"
                    />
                    <Input
                      id="merge_threshold"
                      type="number"
                      min={2}
                      max={500}
                      className="mt-2"
                      disabled={!enabled || !deliveryEnabled}
                      {...form.register("merge_threshold", { valueAsNumber: true })}
                    />
                    {form.formState.errors.merge_threshold && (
                      <p className="text-destructive mt-1 text-xs">
                        {form.formState.errors.merge_threshold.message}
                      </p>
                    )}
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </form>
      )}
    </PageLayout>
  );
}
