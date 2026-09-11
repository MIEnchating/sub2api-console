import type { ReactElement } from "react";
import { BellRing, Settings2 } from "lucide-react";
import { Controller, type UseFormReturn } from "react-hook-form";
import type { NotificationStatus } from "@/api";
import { FieldLabel } from "@/components/field-help-tooltip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { notificationTargetTypeLabels, recoveryNotificationFields } from "../constants";
import type { AlertPolicyFormValues } from "../lib/alert-policy-schema";
import { updateSelection } from "../lib/update-selection";
import { SettingSwitch } from "./setting-switch";

function notificationTargetTypeLabel(channelType: string): string {
  return notificationTargetTypeLabels[channelType] ?? "未知目标";
}

export function NotificationSettings(props: {
  form: UseFormReturn<AlertPolicyFormValues>;
  enabled: boolean;
  notification: NotificationStatus | undefined;
  onOpenSettings: () => void;
}): ReactElement {
  const form = props.form;
  const enabled = props.enabled;
  const deliveryEnabled = form.watch("delivery_enabled");
  const notifyRecovery = form.watch("notify_recovery");
  return (
    <Card role="region" aria-label="通知发送">
      <CardHeader className="flex flex-wrap items-center justify-between gap-2">
        <CardTitle>
          <h2 className="flex items-center gap-2">
            <BellRing className="text-primary size-4" aria-hidden="true" />
            通知发送
          </h2>
        </CardTitle>
        <CardAction className="flex items-center gap-1.5" data-slot="notification-channel-summary">
          <Badge variant={props.notification?.configured ? "secondary" : "warning"}>
            {props.notification?.configured
              ? `QQBot · ${notificationTargetTypeLabel(props.notification.channel_type)}`
              : "QQBot 未配置"}
          </Badge>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
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
      <CardContent className="py-3">
        <div className="grid gap-x-5 sm:grid-cols-2" data-slot="alert-delivery-switches">
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
        <div
          className="mt-3 grid gap-4 border-t pt-3 sm:grid-cols-2 2xl:grid-cols-3"
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
        <div
          className="border-border/70 bg-muted/20 mt-3 rounded-lg border px-3 py-2"
          data-slot="recovery-notification-types"
        >
          <FieldLabel
            label="恢复通知类型"
            description="默认关闭容易频繁波动或无需闭环确认的恢复消息，告警记录仍会正常更新。"
            className="text-sm"
          />
          <div className="mt-1 grid gap-x-5 sm:grid-cols-2">
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
      </CardContent>
    </Card>
  );
}
