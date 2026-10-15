import { useMemo, useState, type ReactElement, type ReactNode } from "react";
import { MultiSelect } from "@/components/multi-select";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { parseOnboardingProbeModels } from "../lib/onboarding-probe-models";

export function OnboardingProbeModelField(props: {
  models?: string[];
  inheritance?: ReactNode;
  fieldId: string;
  identity: string;
  value: string;
  error?: string;
  disabled: boolean;
  onChange: (value: string) => void;
  onBlur: () => void;
}): ReactElement {
  const [manual, setManual] = useState(false);
  const selected = useMemo(() => parseOnboardingProbeModels(props.value), [props.value]);
  const options = useMemo(
    () => (props.models ?? []).map((model) => ({ value: model, label: model })),
    [props.models],
  );
  const errorId = `${props.fieldId}-error`;
  const helpId = `${props.fieldId}-help`;
  return (
    <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 border-t pt-3">
      <label htmlFor={props.fieldId} className="self-center text-sm font-medium">
        探活模型
      </label>
      <div className="order-1 w-full min-w-0">
        {manual ? (
          <Textarea
            id={props.fieldId}
            aria-label={`${props.identity} 探活模型`}
            value={props.value}
            onChange={(event) => props.onChange(event.target.value)}
            onBlur={props.onBlur}
            rows={1}
            disabled={props.disabled}
            aria-invalid={Boolean(props.error)}
            aria-describedby={props.error ? errorId : helpId}
            placeholder="每行一个模型，留空使用默认"
            className="h-8 min-h-8 field-sizing-fixed resize-none overflow-y-auto py-1 leading-5"
          />
        ) : (
          <MultiSelect
            id={props.fieldId}
            options={options}
            selected={selected}
            onChange={(models) => props.onChange(models.join("\n"))}
            title="默认模型"
            ariaLabel={`${props.identity} 探活模型`}
            ariaInvalid={Boolean(props.error)}
            ariaDescribedBy={props.error ? errorId : helpId}
            disabled={props.disabled || (options.length === 0 && selected.length === 0)}
            searchPlaceholder="搜索探活模型"
            clearText="清空，使用默认模型"
            maxVisibleChips={2}
            className="h-8 min-h-8 content-start overflow-y-auto [&_[data-slot=combobox-chip]]:max-w-full"
          />
        )}
      </div>
      <div
        role="group"
        aria-label={`${props.identity} 探活模型填写方式`}
        className="bg-muted/60 flex max-w-full flex-wrap gap-0.5 rounded-lg p-0.5"
      >
        <Button
          type="button"
          variant={manual ? "ghost" : "outline"}
          disabled={props.disabled}
          aria-pressed={!manual}
          onClick={() => setManual(false)}
        >
          从列表选择
        </Button>
        <Button
          type="button"
          variant={manual ? "outline" : "ghost"}
          disabled={props.disabled}
          aria-pressed={manual}
          onClick={() => setManual(true)}
        >
          手动填写
        </Button>
      </div>
      <p id={helpId} className="text-muted-foreground order-2 w-full text-xs">
        仅用于后续探活；留空使用默认探活模型。
      </p>
      {!selected.length && props.inheritance ? (
        <div className="order-3 w-full min-w-0">{props.inheritance}</div>
      ) : null}
      {props.error ? (
        <p id={errorId} className="text-destructive order-4 w-full text-xs">
          {props.error}
        </p>
      ) : null}
    </div>
  );
}
