import type { ReactElement } from "react";
import { Plus, SlidersHorizontal, Trash2 } from "lucide-react";
import { Controller, useFieldArray, type UseFormReturn } from "react-hook-form";
import { FieldLabel } from "@/components/field-help-tooltip";
import { MultiSelect } from "@/components/multi-select";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { AlertPolicyFormValues } from "../lib/alert-policy-schema";

function probeGroupSelectTitle(isLoading: boolean, isError: boolean): string {
  if (isLoading) return "正在读取分组";
  if (isError) return "分组读取失败";
  return "全部分组";
}

export function ThresholdSettings(props: {
  form: UseFormReturn<AlertPolicyFormValues>;
  enabled: boolean;
  groupOptions: Array<{ value: string; label: string }>;
  groupsLoading: boolean;
  groupsError: boolean;
}): ReactElement {
  const form = props.form;
  const enabled = props.enabled;
  const balanceEnabled = form.watch("balance_enabled");
  const probeEnabled = form.watch("probe_enabled");
  const balanceThresholds = useFieldArray({ control: form.control, name: "balance_thresholds" });
  return (
    <Card role="region" aria-label="阈值与范围">
      <CardHeader className="border-b">
        <CardTitle>
          <h2 className="flex items-center gap-2">
            <SlidersHorizontal className="text-primary size-4" aria-hidden="true" />
            阈值与范围
          </h2>
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3 py-3 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <FieldLabel
              label="余额告警阈值"
              description="支持多个提醒档位，例如 20、10、5；余额达到或低于下一档时会再次告警。"
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!enabled || !balanceEnabled || balanceThresholds.fields.length >= 20}
              onClick={() => balanceThresholds.append({ value: "" })}
            >
              <Plus /> 添加阈值
            </Button>
          </div>
          <div className="mt-2 flex flex-wrap items-start gap-2" data-slot="balance-threshold-list">
            {balanceThresholds.fields.map((field, index) => (
              <div className="w-28 max-w-full" key={field.id}>
                <div className="relative">
                  <Input
                    id={`balance-threshold-${index}`}
                    className="pr-9"
                    aria-label={`余额告警阈值 ${index + 1}`}
                    aria-invalid={Boolean(form.formState.errors.balance_thresholds?.[index]?.value)}
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
                    disabled={!enabled || !balanceEnabled || balanceThresholds.fields.length === 1}
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
          />
          <Controller
            control={form.control}
            name="probe_groups"
            render={({ field }) => (
              <MultiSelect
                options={props.groupOptions}
                selected={field.value}
                onChange={field.onChange}
                title={probeGroupSelectTitle(props.groupsLoading, props.groupsError)}
                searchPlaceholder="搜索告警分组"
                emptyText="没有匹配的分组"
                clearText="应用于全部分组"
                ariaLabel="主动探测告警分组"
                disabled={!enabled || !probeEnabled || props.groupsLoading || props.groupsError}
                className="mt-2"
                maxVisibleChips={4}
              />
            )}
          />
        </div>
      </CardContent>
    </Card>
  );
}
