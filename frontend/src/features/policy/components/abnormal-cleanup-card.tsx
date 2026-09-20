import { useId, type ReactElement } from "react";

import { FormField } from "@/components/form-field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { policyCleanupActionLabel, policyCleanupActionOptions } from "../constants";
import { PolicyConfigCard } from "./policy-config-card";
import { PolicySwitchRow } from "./policy-switch-row";

const actions = policyCleanupActionOptions.filter((option) => option.value !== "none");
const numberFields = [
  {
    key: "duration_minutes",
    label: "异常持续时长（天）",
    fallback: 1440,
    max: 365,
    scale: 1440,
    step: "any",
    error: "请输入大于 0 且不超过 365 的天数，时长需为整分钟",
  },
  {
    key: "max_per_round",
    label: "长期异常每轮最多处置（个）",
    fallback: 1,
    max: 10000,
    scale: 1,
    step: 1,
    error: "请输入 1 到 10000 的整数",
  },
] as const;

function scaleInput(value: string, scale: number): number | null {
  if (value === "") return null;
  const scaled = Number(value) * scale;
  const integer = Math.round(scaled);
  // Decimal days can introduce floating-point noise when converted to minutes.
  const tolerance = Number.EPSILON * Math.max(1, Math.abs(scaled)) * 4;
  return Math.abs(scaled - integer) <= tolerance ? integer : scaled;
}

export function AbnormalCleanupCard(props: {
  value: (key: string) => unknown;
  onChange: (key: string, value: unknown) => void;
}): ReactElement {
  const id = useId();
  return (
    <PolicyConfigCard
      title="长期异常账号处置"
      description="持续降级或熔断且仍有新鲜失败证据的账号达到时长后自动处置，默认关闭。成功请求、证据过期或人工保护会重置计时；429 限流和成本拦截不计入。仅完全模式执行。"
      columns={3}
      wide
      switchAction={{
        checked: props.value("enabled") === true,
        label: "启用长期异常账号处置",
        onCheckedChange: (value) => props.onChange("enabled", value),
      }}
    >
      <FormField label="长期异常处置动作">
        <Select
          value={String(props.value("action") ?? "pause")}
          itemToStringLabel={policyCleanupActionLabel}
          onValueChange={(value) => value && props.onChange("action", value)}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {actions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FormField>
      {numberFields.map((field) => {
        const configured = props.value(field.key);
        const value = configured === undefined ? field.fallback : configured;
        const invalid =
          typeof value !== "number" ||
          !Number.isInteger(value) ||
          value < 1 ||
          value > field.max * field.scale;
        return (
          <FormField key={field.key} label={field.label}>
            <Input
              aria-label={field.label}
              aria-invalid={invalid}
              aria-describedby={invalid ? `${id}-${field.key}-error` : undefined}
              type="number"
              min={1 / field.scale}
              max={field.max}
              step={field.step}
              value={typeof value === "number" ? value / field.scale : ""}
              onChange={(event) =>
                props.onChange(field.key, scaleInput(event.target.value, field.scale))
              }
            />
            {invalid ? (
              <span id={`${id}-${field.key}-error`} className="text-destructive text-xs">
                {field.error}
              </span>
            ) : null}
          </FormField>
        );
      })}
      <PolicySwitchRow
        label="长期异常处置保留分组最后一个账号"
        description="与认证失效处置共同检查本轮影响，避免清空分组。"
        checked={props.value("keep_last_in_group") !== false}
        onCheckedChange={(value) => props.onChange("keep_last_in_group", value)}
      />
    </PolicyConfigCard>
  );
}
