import type { GroupPolicyOverrideUpdate } from "../../../api";
import { Button } from "../../../components/ui/button";
import { Input } from "../../../components/ui/input";
import { Switch } from "../../../components/ui/switch";
import { cn } from "../../../lib/utils";
import {
  schedulingStrategyDescription,
  schedulingStrategyOptions,
  schedulingWeightFormula,
} from "../../../lib/scheduling-strategy";

const capabilityOptions = [
  ["breaker_enabled", "熔断"],
  ["recovery_enabled", "健康回池"],
  ["weights_enabled", "负载因子调权"],
  ["scaling_enabled", "智能扩容"],
] as const;

export const groupPolicyDialogLayout = {
  content: "grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden",
  body: "min-h-0 overflow-x-hidden overflow-y-auto overscroll-contain pr-1",
} as const;

export function GroupPolicyEditorFields(props: {
  value: GroupPolicyOverrideUpdate;
  onChange: (value: GroupPolicyOverrideUpdate) => void;
}) {
  const update = <Key extends keyof GroupPolicyOverrideUpdate>(
    field: Key,
    value: GroupPolicyOverrideUpdate[Key],
  ) => props.onChange({ ...props.value, [field]: value });

  return (
    <div className="min-w-0 space-y-5" data-testid="group-policy-editor-fields">
      <div className="flex min-w-0 items-center justify-between gap-4 border-b pb-4">
        <label className="min-w-0 cursor-pointer" htmlFor="group-policy-enabled">
          <span className="block text-sm font-medium">参与守护</span>
          <span className="text-muted-foreground block text-xs">关闭后不探测、不熔断、不调权</span>
        </label>
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
        <label className="min-w-0 space-y-1.5 text-sm">
          <span className="block font-medium">保底可用账号数</span>
          <Input
            className="min-w-0"
            type="number"
            min={0}
            value={props.value.min_pool_size}
            onChange={(event) => update("min_pool_size", Number(event.target.value))}
          />
        </label>
        <label className="min-w-0 space-y-1.5 text-sm">
          <span className="block font-medium">组内总权重预算</span>
          <Input
            className="min-w-0"
            type="number"
            min={1}
            value={props.value.weight_budget}
            onChange={(event) => update("weight_budget", Number(event.target.value))}
          />
          <span className="text-muted-foreground block text-xs font-normal">
            由同组参与调度的账号按策略共享
          </span>
        </label>
        <label className="min-w-0 space-y-1.5 text-sm">
          <span className="block font-medium">均衡策略价格占比</span>
          <Input
            className="min-w-0"
            type="number"
            min={0}
            max={1}
            step={0.05}
            value={props.value.balanced_price_ratio}
            onChange={(event) => update("balanced_price_ratio", Number(event.target.value))}
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
          {capabilityOptions.map(([field, label]) => (
            <div key={field} className="flex min-w-0 items-center justify-between gap-4">
              <label
                className="min-w-0 cursor-pointer text-sm font-medium"
                htmlFor={`group-policy-${field}`}
              >
                {label}
              </label>
              <Switch
                id={`group-policy-${field}`}
                checked={props.value[field]}
                onCheckedChange={(checked) => update(field, checked)}
              />
            </div>
          ))}
        </div>
      </section>

      <section className="min-w-0 space-y-3" data-testid="group-policy-probe-settings">
        <div className="flex min-w-0 items-center justify-between gap-4">
          <label className="min-w-0 cursor-pointer" htmlFor="group-policy-probe-enabled">
            <span className="block text-sm font-medium">定时测试</span>
            <span className="text-muted-foreground block text-xs">
              定期测试该分组账号，测试参数仅覆盖当前分组。
            </span>
          </label>
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
              value={props.value.probe_interval_seconds}
              disabled={!props.value.probe_enabled}
              onChange={(event) => update("probe_interval_seconds", Number(event.target.value))}
            />
          </label>
          <label className="min-w-0 space-y-1.5 text-sm">
            <span className="block font-medium">测试模型</span>
            <Input
              className="min-w-0"
              value={props.value.probe_model ?? ""}
              disabled={!props.value.probe_enabled}
              placeholder="留空使用全局默认"
              onChange={(event) => update("probe_model", event.target.value || null)}
            />
          </label>
        </div>
      </section>
    </div>
  );
}
