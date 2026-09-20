import { useId, type ReactElement } from "react";

import { FieldError } from "@/components/field-error";
import { FieldLabel } from "@/components/field-help-tooltip";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";

type ServiceSettingsProps = {
  enabled: boolean;
  intervalSeconds: number | null;
  disabled: boolean;
  error?: string;
  onEnabledChange: (enabled: boolean) => void;
  onIntervalChange: (interval: number | null) => void;
};

export function ServiceSettings(props: ServiceSettingsProps): ReactElement {
  const enabledId = useId();
  const intervalId = useId();
  const errorId = `${intervalId}-error`;

  return (
    <div className="grid min-w-0 gap-3 sm:grid-cols-2" data-testid="auto-inspection-settings">
      <div className="flex min-h-8 min-w-0 items-center justify-between gap-3 sm:border-r sm:pr-4">
        <FieldLabel
          label="启用自动巡检"
          htmlFor={enabledId}
          description="开启后由后台持续检查到期任务，不依赖浏览器登录状态。"
        />
        <Switch
          id={enabledId}
          aria-label="启用自动巡检"
          checked={props.enabled}
          disabled={props.disabled}
          onCheckedChange={props.onEnabledChange}
        />
      </div>
      <div className="min-w-0">
        <div className="flex min-h-8 items-center justify-between gap-3">
          <FieldLabel
            label="调度心跳"
            htmlFor={intervalId}
            description="每次心跳先同步管理端账号与分组，再执行到期的巡检任务。"
          />
          <div className="flex shrink-0 items-center gap-2">
            <Input
              id={intervalId}
              className="w-24 tabular-nums"
              type="number"
              min={15}
              max={86400}
              value={props.intervalSeconds ?? ""}
              aria-label="调度心跳周期"
              aria-invalid={Boolean(props.error)}
              aria-describedby={props.error ? errorId : undefined}
              disabled={props.disabled}
              onChange={(event) =>
                props.onIntervalChange(
                  event.target.value === "" ? null : Number(event.target.value),
                )
              }
            />
            <span className="text-muted-foreground text-sm">秒</span>
          </div>
        </div>
        {props.error ? (
          <FieldError id={errorId} message={props.error} className="mt-1 h-auto min-h-4" />
        ) : null}
      </div>
    </div>
  );
}
