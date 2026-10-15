import { useId, type ReactElement } from "react";

import { FormField } from "@/components/form-field";
import { Input } from "@/components/ui/input";
import { PolicyConfigCard } from "./policy-config-card";

export function BindingCleanupCard(props: {
  value: (key: string) => unknown;
  onChange: (key: string, value: unknown) => void;
}): ReactElement {
  const errorId = useId();
  const configured = props.value("max_per_round");
  const value = configured === undefined ? 1 : configured;
  const invalid =
    typeof value !== "number" || !Number.isInteger(value) || value < 1 || value > 10000;
  return (
    <PolicyConfigCard
      title="失效绑定账号自动删除"
      description="连续完整同步确认上游 Key 或所属分组不存在后，同步删除上游 Key 和 Sub2API 中对应的受管账号；Key 已不存在时继续清理账号，Key 删除或读回失败时保留账号。默认关闭，仅完全模式执行；跳过人工保护、绑定不明确及同步失败的账号，删除前再次复核。允许删除分组最后一个失效账号。"
      wide
      switchAction={{
        checked: props.value("enabled") === true,
        label: "启用失效绑定账号自动删除",
        onCheckedChange: (value) => props.onChange("enabled", value),
      }}
    >
      <FormField label="失效绑定每轮最多删除（个）">
        <Input
          aria-label="失效绑定每轮最多删除（个）"
          aria-invalid={invalid}
          aria-describedby={invalid ? errorId : undefined}
          type="number"
          min={1}
          max={10000}
          step={1}
          value={typeof value === "number" ? value : ""}
          onChange={(event) =>
            props.onChange(
              "max_per_round",
              event.target.value === "" ? null : Number(event.target.value),
            )
          }
        />
        {invalid ? (
          <span id={errorId} className="text-destructive text-xs">
            请输入 1 到 10000 的整数
          </span>
        ) : null}
      </FormField>
    </PolicyConfigCard>
  );
}
