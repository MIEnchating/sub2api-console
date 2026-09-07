import type { GroupPolicyOverrideUpdate, GroupProbeModels } from "../../../api";
import { RefreshCw } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../components/ui/button";
import { FieldLabel } from "../../../components/field-help-tooltip";
import { Input } from "../../../components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../../../components/ui/select";
import { Switch } from "../../../components/ui/switch";
import { cn } from "../../../lib/utils";
import {
  schedulingStrategyDescription,
  schedulingStrategyOptions,
  schedulingWeightFormula,
} from "../../../lib/scheduling-strategy";

export type GroupPolicyOverrideDraft = Omit<
  GroupPolicyOverrideUpdate,
  "min_pool_size" | "weight_budget" | "balanced_price_ratio" | "probe_interval_seconds"
> & {
  min_pool_size: number | null;
  weight_budget: number | null;
  balanced_price_ratio: number | null;
  probe_interval_seconds: number | null;
};

const capabilityOptions = [
  {
    field: "breaker_enabled",
    label: "自动熔断",
    description: "故障达到条件后自动停止调度；开启健康回池后可自动恢复",
  },
  {
    field: "recovery_enabled",
    label: "健康回池",
    description: "熔断账号连续探测通过后重新参与调度",
  },
  {
    field: "weights_enabled",
    label: "负载因子调权",
    description: "按健康、延迟和成本调整账号负载因子",
  },
  {
    field: "scaling_enabled",
    label: "智能扩容",
    description: "按实际流量提高或降低账号并发上限",
  },
] as const;

export const groupPolicyDialogLayout = {
  content: "grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden",
  body: "min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain pr-1",
} as const;

export function groupProbeModelOptions(models: string[], currentModel: string | null): string[] {
  const options = new Map<string, string>();
  for (const rawModel of [...models, currentModel ?? ""]) {
    const model = rawModel.trim();
    if (model.length === 0) continue;
    const key = model.toLocaleLowerCase();
    if (!options.has(key)) options.set(key, model);
  }
  return [...options.values()].sort((left, right) => left.localeCompare(right));
}

export function groupProbeModelDraftValue(model: string | null | undefined): string | null {
  return model?.trim() || null;
}

const inheritedProbeModelValue = "\u0000inherited-probe-model";

function ProbeModelControl(props: {
  value: string | null;
  options: string[];
  disabled: boolean;
  onChange: (value: string | null) => void;
}) {
  const [mode, setMode] = useState<"manual" | "select">("manual");
  const selectedValue = props.value?.trim() || inheritedProbeModelValue;

  return (
    <div className="min-w-0 space-y-1.5">
      <div className="flex min-w-0 items-center justify-between gap-3">
        <span className="block font-medium">探活模型</span>
        <div
          className="bg-muted grid grid-cols-2 rounded-md p-0.5"
          role="group"
          aria-label="探活模型输入方式"
        >
          {(["manual", "select"] as const).map((optionMode) => {
            const selected = mode === optionMode;
            const label = optionMode === "manual" ? "手动输入" : "选择模型";
            return (
              <button
                key={optionMode}
                type="button"
                className={cn(
                  "text-muted-foreground h-6 rounded-sm px-2 text-xs transition-colors outline-none focus-visible:ring-2",
                  selected && "bg-background text-foreground shadow-sm",
                )}
                aria-pressed={selected}
                disabled={props.disabled || (optionMode === "select" && props.options.length === 0)}
                onClick={() => setMode(optionMode)}
              >
                {label}
              </button>
            );
          })}
        </div>
      </div>
      {mode === "manual" ? (
        <Input
          className="min-w-0"
          aria-label="手动输入探活模型"
          value={props.value ?? ""}
          disabled={props.disabled}
          placeholder="留空使用全局默认"
          onChange={(event) => props.onChange(event.target.value || null)}
        />
      ) : (
        <Select
          value={selectedValue}
          itemToStringLabel={(value) =>
            value === inheritedProbeModelValue ? "继承全局默认模型" : value
          }
          disabled={props.disabled}
          onValueChange={(value) => {
            if (!value) return;
            props.onChange(value === inheritedProbeModelValue ? null : value);
          }}
        >
          <SelectTrigger aria-label="选择探活模型">
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="start">
            <SelectItem value={inheritedProbeModelValue}>继承全局默认模型</SelectItem>
            {props.options.map((model) => (
              <SelectItem key={model} value={model}>
                {model}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}

export function GroupPolicyEditorFields(props: {
  value: GroupPolicyOverrideDraft;
  onChange: (value: GroupPolicyOverrideDraft) => void;
  probeModels?: GroupProbeModels;
  probeModelsLoading?: boolean;
  onReloadProbeModels?: () => void;
}) {
  const update = (
    field: keyof GroupPolicyOverrideDraft,
    value: GroupPolicyOverrideDraft[keyof GroupPolicyOverrideDraft],
  ) => props.onChange({ ...props.value, [field]: value });
  const probeModelOptions = groupProbeModelOptions(
    props.probeModels?.models ?? [],
    props.value.probe_model,
  );
  let probeModelsButtonLabel = props.probeModels ? "重新获取组内模型" : "获取组内模型";
  if (props.probeModelsLoading) probeModelsButtonLabel = "正在获取";

  return (
    <div className="min-w-0 space-y-5" data-testid="group-policy-editor-fields">
      <div className="flex min-w-0 items-center justify-between gap-4 border-b pb-4">
        <FieldLabel
          label="参与守护"
          description="关闭后不探测、不熔断、不调权"
          htmlFor="group-policy-enabled"
          className="cursor-pointer text-sm"
        />
        <Switch
          id="group-policy-enabled"
          checked={props.value.enabled}
          onCheckedChange={(enabled) => update("enabled", enabled)}
        />
      </div>

      <fieldset className="min-w-0 space-y-2">
        <legend className="text-sm font-medium">调度策略</legend>
        <div
          className="grid min-w-0 grid-cols-2 gap-2 sm:grid-cols-4"
          role="radiogroup"
          aria-label="调度策略"
          data-testid="group-policy-strategy-options"
        >
          {schedulingStrategyOptions.map((strategy) => {
            const selected = props.value.strategy === strategy.value;
            return (
              <Button
                key={strategy.value}
                type="button"
                variant="outline"
                className={cn(
                  "h-9 w-full min-w-0 rounded-lg border",
                  selected &&
                    "border-primary bg-primary text-primary-foreground hover:bg-primary/90 hover:text-primary-foreground dark:bg-primary dark:hover:bg-primary/90",
                )}
                role="radio"
                aria-checked={selected}
                onClick={() => update("strategy", strategy.value)}
              >
                {strategy.label}
              </Button>
            );
          })}
        </div>
        <p className="text-muted-foreground text-xs leading-5">
          {schedulingStrategyDescription(props.value.strategy)}；{schedulingWeightFormula}
        </p>
      </fieldset>

      <div className="grid min-w-0 gap-4 sm:grid-cols-3">
        <div className="min-w-0 space-y-1.5 text-sm">
          <FieldLabel
            label="保底可用账号数"
            description="即使账号达到熔断条件，每个分组仍至少保留这么多个账号接收请求，避免整个分组中断"
            htmlFor="group-policy-min-pool-size"
          />
          <Input
            id="group-policy-min-pool-size"
            className="min-w-0"
            type="number"
            min={0}
            value={props.value.min_pool_size ?? ""}
            onChange={(event) =>
              update("min_pool_size", event.target.value === "" ? null : Number(event.target.value))
            }
          />
        </div>
        <div className="min-w-0 space-y-1.5 text-sm">
          <FieldLabel
            label="组内总权重预算"
            description="由同组参与调度的账号按策略共享"
            htmlFor="group-policy-weight-budget"
          />
          <Input
            id="group-policy-weight-budget"
            className="min-w-0"
            type="number"
            min={1}
            value={props.value.weight_budget ?? ""}
            onChange={(event) =>
              update("weight_budget", event.target.value === "" ? null : Number(event.target.value))
            }
          />
        </div>
        <label className="min-w-0 space-y-1.5 text-sm">
          <span className="block font-medium">均衡策略价格占比</span>
          <Input
            className="min-w-0"
            type="number"
            min={0}
            max={1}
            step={0.05}
            value={props.value.balanced_price_ratio ?? ""}
            onChange={(event) =>
              update(
                "balanced_price_ratio",
                event.target.value === "" ? null : Number(event.target.value),
              )
            }
          />
        </label>
      </div>

      <section className="min-w-0 space-y-3 border-y py-4">
        <div>
          <h3 className="text-sm font-medium">策略能力</h3>
          <p className="text-muted-foreground mt-0.5 text-xs">控制该分组参与的自动调度能力。</p>
        </div>
        <div
          className="grid min-w-0 gap-x-8 gap-y-3 sm:grid-cols-2"
          data-testid="group-policy-capability-switches"
        >
          {capabilityOptions.map((option) => (
            <div key={option.field} className="flex min-w-0 items-center justify-between gap-4">
              <FieldLabel
                label={option.label}
                description={option.description}
                htmlFor={`group-policy-${option.field}`}
                className="cursor-pointer text-sm"
              />
              <Switch
                id={`group-policy-${option.field}`}
                checked={props.value[option.field]}
                onCheckedChange={(checked) => update(option.field, checked)}
              />
            </div>
          ))}
        </div>
      </section>

      <section className="min-w-0 space-y-3" data-testid="group-policy-probe-settings">
        <div className="flex min-w-0 items-center justify-between gap-4">
          <FieldLabel
            label="定时测试"
            description="定期测试该分组账号，测试参数仅覆盖当前分组。"
            htmlFor="group-policy-probe-enabled"
            className="cursor-pointer text-sm"
          />
          <Switch
            id="group-policy-probe-enabled"
            checked={props.value.probe_enabled}
            onCheckedChange={(checked) => update("probe_enabled", checked)}
          />
        </div>
        <div className="grid min-w-0 gap-4 sm:grid-cols-2">
          <label className="min-w-0 space-y-1.5 text-sm">
            <span className="block font-medium">测试间隔（秒）</span>
            <Input
              className="min-w-0"
              type="number"
              min={30}
              value={props.value.probe_interval_seconds ?? ""}
              disabled={!props.value.probe_enabled}
              onChange={(event) =>
                update(
                  "probe_interval_seconds",
                  event.target.value === "" ? null : Number(event.target.value),
                )
              }
            />
          </label>
          <div className="min-w-0 space-y-1.5 text-sm">
            <div className="grid min-w-0 gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
              <ProbeModelControl
                options={probeModelOptions}
                value={props.value.probe_model}
                disabled={!props.value.probe_enabled}
                onChange={(model) => update("probe_model", model)}
              />
              <Button
                type="button"
                variant="outline"
                className="whitespace-nowrap"
                disabled={
                  !props.value.probe_enabled ||
                  props.probeModelsLoading ||
                  !props.onReloadProbeModels
                }
                onClick={props.onReloadProbeModels}
              >
                <RefreshCw className={props.probeModelsLoading ? "animate-spin" : undefined} />
                {probeModelsButtonLabel}
              </Button>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
