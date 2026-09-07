import type { ReactElement } from "react";
import { ShieldAlert } from "lucide-react";
import { Controller, type UseFormReturn } from "react-hook-form";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { alertRuleGroups, routingDegradedFields } from "../constants";
import type { AlertPolicyFormValues } from "../lib/alert-policy-schema";
import { updateSelection } from "../lib/update-selection";
import { SettingSwitch } from "./setting-switch";

export function DetectionSettings(props: {
  form: UseFormReturn<AlertPolicyFormValues>;
  enabled: boolean;
}): ReactElement {
  const form = props.form;
  const enabled = props.enabled;
  const routingDegradedEnabled = form.watch("routing_degraded_enabled");
  return (
    <Card role="region" aria-label="告警检测">
      <CardHeader>
        <CardTitle>
          <h2 className="flex items-center gap-2">
            <ShieldAlert className="text-primary size-4" aria-hidden="true" />
            告警检测
          </h2>
        </CardTitle>
      </CardHeader>
      <CardContent className="py-3">
        <div className="bg-primary/5 border-primary/15 mb-4 rounded-lg border px-3 py-1">
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
        </div>
        <h3 className="mb-3 text-sm font-medium">检测规则</h3>

        <div className="grid gap-3" data-slot="alert-rule-grid">
          {alertRuleGroups.map((group) => (
            <section
              key={group.label}
              data-slot="alert-rule-group"
              className="border-border/70 bg-muted/20 rounded-lg border px-3 py-1.5"
            >
              <h3 className="text-muted-foreground mb-1 text-xs font-medium">{group.label}</h3>
              <div className="grid gap-x-5 sm:grid-cols-2">
                {group.fields.map((rule) => (
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
            </section>
          ))}
        </div>
        <div
          className="border-border/70 bg-muted/20 mt-3 rounded-lg border px-3 py-1.5"
          data-slot="routing-degraded-rules"
        >
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
          <div className="border-border/70 mt-1 grid gap-x-5 border-t pt-1 sm:grid-cols-2">
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
  );
}
