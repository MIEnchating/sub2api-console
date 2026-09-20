import { Switch } from "@/components/ui/switch";
import { FieldLabel } from "@/components/field-help-tooltip";

export function AccountCostWallSwitch(props: {
  accountId: string;
  enabled: boolean;
  onCheckedChange: (enabled: boolean) => void;
  disabled?: boolean;
  compact?: boolean;
}) {
  const id = `account-ignore-cost-wall-${props.accountId}`;
  const description =
    "默认关闭。开启后，成本墙不再限制此账号的调度、权重和自动探活，并停止无利润／亏损流量告警及其恢复通知；仍遵守其他调度规则。点击保存后生效，后续调度按新设置评估。";
  return (
    <div
      data-slot="settings-switch-row"
      className="flex min-h-12 items-center justify-between gap-3 px-3 py-2"
    >
      <div className="grid min-w-0 gap-1">
        <FieldLabel
          htmlFor={id}
          label="无视成本墙"
          description={props.compact ? description : undefined}
          className="text-sm"
        />
        <p
          id={`${id}-description`}
          className={props.compact ? "sr-only" : "text-muted-foreground text-xs leading-4"}
        >
          {description}
        </p>
        {props.compact ? (
          <p className="text-muted-foreground text-xs leading-5">忽略成本限制，停止相关流量告警</p>
        ) : null}
      </div>
      <Switch
        id={id}
        checked={props.enabled}
        disabled={props.disabled}
        aria-describedby={`${id}-description`}
        className="shrink-0"
        onCheckedChange={props.onCheckedChange}
      />
    </div>
  );
}
