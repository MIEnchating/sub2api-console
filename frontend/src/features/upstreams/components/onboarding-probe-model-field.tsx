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
    <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-2 border-t pt-4">
      <label htmlFor={props.fieldId} className="self-center text-sm font-medium">
        探活模型
      </label>
      <div className="col-span-2 row-start-2 min-w-0">
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
        className="col-start-2 row-start-1 flex gap-2"
      >
        <Button
          type="button"
          variant={manual ? "outline" : "secondary"}
          disabled={props.disabled}
          aria-pressed={!manual}
          onClick={() => setManual(false)}
        >
          从列表选择
        </Button>
        <Button
          type="button"
          variant={manual ? "secondary" : "outline"}
          disabled={props.disabled}
          aria-pressed={manual}
          onClick={() => setManual(true)}
        >
          手动填写
        </Button>
      </div>
      <p id={helpId} className="text-muted-foreground col-span-2 text-xs">
        仅用于后续探活，可多选或每行填写一个模型；留空使用默认探活模型。
      </p>
      {!selected.length && props.inheritance ? (
        <div className="col-span-2 min-w-0">{props.inheritance}</div>
      ) : null}
      {props.error ? (
        <p id={errorId} className="text-destructive col-span-2 text-xs">
          {props.error}
        </p>
      ) : null}
    </div>
  );
}
