import { ProbeModelInheritance } from "@/components/probe-model-inheritance";
import { RefreshCw } from "lucide-react";
import { useEffect, useId, type ReactElement } from "react";
import { Controller, type UseFormReturn } from "react-hook-form";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { useOnboardingModelOptions } from "../hooks/use-onboarding-model-options";
import type { OnboardingConfirmationForm } from "../lib/onboarding-model-mapping";
import type { OnboardingBindingPreview } from "./onboarding-confirm-dialog";
import { OnboardingModelMappingFields } from "./onboarding-model-mapping-fields";
import { OnboardingProbeModelField } from "./onboarding-probe-model-field";

export function OnboardingAccountModelFields(props: {
  form: UseFormReturn<OnboardingConfirmationForm>;
  index: number;
  item: OnboardingBindingPreview;
  disabled: boolean;
  onPendingChange: (accountId: string, pending: boolean) => void;
}): ReactElement {
  const source = useOnboardingModelOptions(props.item.host, props.item.upstreamGroupId);
  const fieldId = useId();
  useEffect(() => {
    props.onPendingChange(props.item.id, source.pending);
    return () => props.onPendingChange(props.item.id, false);
  }, [props.item.id, props.onPendingChange, source.pending]);
  return (
    <>
      <div className="grid min-w-0 gap-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-muted-foreground text-xs">
            {source.loaded
              ? `已获取 ${source.models.length} 个上游模型`
              : "获取模型后，可用于下方映射和探活选择。"}
          </p>
          {!source.loaded ? (
            <Button
              type="button"
              variant="outline"
              disabled={props.disabled || source.pending}
              onClick={source.load}
            >
              <RefreshCw aria-hidden="true" />
              获取模型
            </Button>
          ) : null}
        </div>
        {source.pending ? <ContentLoading compact label="正在获取上游模型" /> : null}
        {source.failed && !source.loaded ? (
          <ContentRetry onRetry={source.load} pending={source.pending || props.disabled} />
        ) : null}
      </div>
      <OnboardingModelMappingFields
        form={props.form}
        index={props.index}
        disabled={props.disabled}
        models={source.models}
        modelsPending={source.pending}
      />
      <Controller
        control={props.form.control}
        name={`accounts.${props.index}.models`}
        render={(controller) => (
          <OnboardingProbeModelField
            inheritance={
              props.item.localGroupIds ? (
                <ProbeModelInheritance groupIds={props.item.localGroupIds} />
              ) : undefined
            }
            fieldId={fieldId}
            identity={`${props.item.upstreamGroup} → ${props.item.localGroup}`}
            models={source.models}
            value={controller.field.value}
            error={controller.fieldState.error?.message}
            disabled={props.disabled}
            onChange={controller.field.onChange}
            onBlur={controller.field.onBlur}
          />
        )}
      />
    </>
  );
}
