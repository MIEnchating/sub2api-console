import { RefreshCw } from "lucide-react";
import { useEffect, useMemo, useState, type ReactElement } from "react";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { MultiSelect } from "@/components/multi-select";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useOnboardingModelOptions } from "../hooks/use-onboarding-model-options";
import { parseOnboardingProbeModels } from "../lib/onboarding-probe-models";

export function OnboardingProbeModelField(props: {
  accountId: string;
  host: string;
  groupId: string;
  fieldId: string;
  identity: string;
  value: string;
  error?: string;
  disabled: boolean;
  onChange: (value: string) => void;
  onBlur: () => void;
  onPendingChange: (accountId: string, pending: boolean) => void;
}): ReactElement {
  const source = useOnboardingModelOptions(props.host, props.groupId);
  const [manual, setManual] = useState(false);
  const selected = useMemo(() => parseOnboardingProbeModels(props.value), [props.value]);
  const options = useMemo(
    () => source.models.map((model) => ({ value: model, label: model })),
    [source.models],
  );
  const errorId = `${props.fieldId}-error`;
  const helpId = `${props.fieldId}-help`;
  useEffect(() => {
    props.onPendingChange(props.accountId, source.pending);
    return () => props.onPendingChange(props.accountId, false);
  }, [props.accountId, props.onPendingChange, source.pending]);
  return (
    <div className="grid min-w-0 gap-1.5 sm:grid-cols-[auto_minmax(0,1fr)] sm:items-start sm:gap-x-3">
      <label htmlFor={props.fieldId} className="text-sm sm:pt-1.5">
        探活模型
      </label>
      <div className="grid min-w-0 gap-1.5">
        <div className="flex min-w-0 items-start gap-2">
          <div className="min-w-0 flex-1">
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
                className="min-h-8 max-h-24 resize-y py-1.5"
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
              />
            )}
          </div>
          <Button
            type="button"
            variant="outline"
            disabled={props.disabled || source.pending}
            onClick={source.load}
            className="shrink-0"
          >
            <RefreshCw aria-hidden="true" />
            {source.loaded ? "重新获取" : "获取模型"}
          </Button>
        </div>
        {source.pending ? <ContentLoading compact label="正在获取上游模型" /> : null}
        {source.failed && !source.loaded ? (
          <ContentRetry onRetry={source.load} pending={source.pending || props.disabled} />
        ) : null}
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-2 gap-y-1">
          <p id={helpId} className="text-muted-foreground text-xs">
            {manual ? "多个模型每行一个" : "获取后可多选，留空使用默认模型"}
          </p>
          <Button
            type="button"
            variant="ghost"
            disabled={props.disabled}
            aria-expanded={manual}
            onClick={() => setManual(!manual)}
            className="shrink-0"
          >
            {manual ? "从列表选择" : "手动填写"}
          </Button>
        </div>
        {props.error ? (
          <p id={errorId} className="text-destructive text-xs">
            {props.error}
          </p>
        ) : null}
      </div>
    </div>
  );
}
