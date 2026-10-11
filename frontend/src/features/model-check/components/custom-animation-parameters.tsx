import type { ReactElement } from "react";
import type { UseFormReturn } from "react-hook-form";
import { FieldError } from "@/components/field-error";
import { Input } from "@/components/ui/input";
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
      className="grid w-full min-w-0 shrink-0 grid-cols-1 items-start gap-3 sm:w-[34rem] sm:max-w-full sm:grid-cols-[minmax(0,1fr)_10rem]"
    >
      <CustomAnimationModelField form={props.form} disabled={props.disabled} />
      <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-2 gap-y-1.5">
        <label
          htmlFor="custom-animation-timeout_seconds"
          className="whitespace-nowrap text-xs font-medium"
        >
          请求超时（秒）
        </label>
        <Input
          id="custom-animation-timeout_seconds"
          type="number"
          min={5}
          max={120}
          disabled={props.disabled}
          {...props.form.register("timeout_seconds", { valueAsNumber: true })}
          aria-invalid={!!props.form.formState.errors.timeout_seconds}
          aria-describedby={
            props.form.formState.errors.timeout_seconds
              ? "custom-animation-timeout_seconds-error"
              : undefined
          }
        />
        {props.form.formState.errors.timeout_seconds ? (
          <FieldError
            className="col-start-2"
            id="custom-animation-timeout_seconds-error"
            message={props.form.formState.errors.timeout_seconds.message}
          />
        ) : null}
      </div>
    </fieldset>
  );
}
