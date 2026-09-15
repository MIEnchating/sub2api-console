import type { ReactElement } from "react";
import { Controller, type UseFormReturn } from "react-hook-form";
import { FieldError } from "@/components/field-error";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { CustomAnimationForm } from "../lib/animation-schema";
import { CustomAnimationModelField } from "./custom-animation-model-field";

const fields = [
  { name: "timeout_seconds", label: "请求超时（秒）", type: "number", placeholder: "120" },
  { name: "base_url", label: "Base URL", type: "url", placeholder: "https://api.example.com/v1" },
  { name: "api_key", label: "API Key", type: "password", placeholder: "输入 API Key" },
  { name: "model", label: "检测模型", type: "text", placeholder: "输入模型 ID" },
] as const;

export function CustomAnimationFields(props: {
  form: UseFormReturn<CustomAnimationForm>;
  disabled: boolean;
}): ReactElement {
  return (
    <fieldset
      disabled={props.disabled}
      className="grid min-w-0 grid-cols-2 gap-x-3 gap-y-2 xl:grid-cols-6"
    >
      <div className="min-w-0 space-y-1">
        <label htmlFor="custom-animation-platform" className="text-sm font-medium">
          接口类型
        </label>
        <Controller
          control={props.form.control}
          name="platform"
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange} disabled={props.disabled}>
              <SelectTrigger id="custom-animation-platform" ref={field.ref} onBlur={field.onBlur}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="openai">OpenAI</SelectItem>
                <SelectItem value="anthropic">Anthropic</SelectItem>
              </SelectContent>
            </Select>
          )}
        />
      </div>
      {fields.map((field) => {
        if (field.name === "model")
          return (
            <CustomAnimationModelField
              key={field.name}
              form={props.form}
              disabled={props.disabled}
            />
          );
        const error = props.form.formState.errors[field.name];
        const id = `custom-animation-${field.name}`;
        return (
          <div key={field.name} className="min-w-0 space-y-1">
            <label htmlFor={id} className="text-sm font-medium">
              {field.label}
            </label>
            <Input
              id={id}
              type={field.type}
              placeholder={field.placeholder}
              autoComplete="off"
              spellCheck={false}
              min={field.type === "number" ? 5 : undefined}
              max={field.type === "number" ? 120 : undefined}
              {...props.form.register(field.name, { valueAsNumber: field.type === "number" })}
              aria-invalid={!!error}
              aria-describedby={error ? `${id}-error` : undefined}
            />
            {error ? <FieldError id={`${id}-error`} message={error.message} /> : null}
          </div>
        );
      })}
    </fieldset>
  );
}
