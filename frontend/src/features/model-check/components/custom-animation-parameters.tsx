import type { ReactElement } from "react";
import type { UseFormReturn } from "react-hook-form";
import { SavedDetectionTimeout } from "./saved-detection-timeout";
import { CustomAnimationModelField } from "./custom-animation-model-field";
import type { CustomAnimationForm } from "../lib/animation-schema";

export function CustomAnimationParameters(props: {
  form: UseFormReturn<CustomAnimationForm>;
  disabled: boolean;
}): ReactElement {
  return (
    <fieldset
      disabled={props.disabled}
      role="group"
      aria-label="检测参数"
      className="grid w-full min-w-0 shrink-0 grid-cols-1 items-start gap-3 sm:max-w-full sm:grid-cols-2 xl:grid-cols-3"
    >
      <CustomAnimationModelField form={props.form} disabled={props.disabled} />
      <SavedDetectionTimeout mode="animation" disabled={props.disabled} />
      <SavedDetectionTimeout mode="precheck" disabled={props.disabled} />
    </fieldset>
  );
}
