import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
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
import { animationPlatforms, animationPlatformLabels } from "../constants";

const platforms = animationPlatforms.map((value) => ({
  value,
  label: animationPlatformLabels[value],
}));

const fields = [
  { name: "base_url", label: "Base URL", type: "url", placeholder: "https://api.example.com/v1" },
  { name: "api_key", label: "API Key", type: "password", placeholder: "输入 API Key" },
] as const;

export function CustomAnimationFields(props: {
  form: UseFormReturn<CustomAnimationForm>;
  disabled: boolean;
}): ReactElement {
  const orderedPlatforms = useDictionaryOrder("platform", platforms, (platform) => platform.value);
  return (
    <fieldset disabled={props.disabled} className="min-w-0">
      <div
        role="group"
        aria-label="接口配置"
        className="grid min-w-0 grid-cols-1 gap-3 lg:grid-cols-[12rem_minmax(0,1.2fr)_minmax(0,1fr)]"
      >
        <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-2 gap-y-1.5">
          <label
            htmlFor="custom-animation-platform"
            className="whitespace-nowrap text-xs font-medium"
          >
            接口类型
          </label>
          <Controller
            control={props.form.control}
            name="platform"
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={field.onChange}
                disabled={props.disabled}
                itemToStringLabel={(value) =>
                  orderedPlatforms.find((platform) => platform.value === value)?.label ??
                  platforms.find((platform) => platform.value === value)?.label ??
                  value
                }
              >
                <SelectTrigger id="custom-animation-platform" ref={field.ref} onBlur={field.onBlur}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent dictionaryKind="platform">
                  {orderedPlatforms.map((platform) => (
                    <SelectItem key={platform.value} value={platform.value}>
                      {platform.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </div>
        {fields.map((field) => {
          const error = props.form.formState.errors[field.name];
          const id = `custom-animation-${field.name}`;
          return (
            <div
              key={field.name}
              className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-2 gap-y-1.5"
            >
              <label htmlFor={id} className="whitespace-nowrap text-xs font-medium">
                {field.label}
              </label>
              <Input
                id={id}
                type={field.type}
                placeholder={field.placeholder}
                autoComplete="off"
                spellCheck={false}
                {...props.form.register(field.name)}
                aria-invalid={!!error}
                aria-describedby={error ? `${id}-error` : undefined}
              />
              {error ? (
                <FieldError className="col-start-2" id={`${id}-error`} message={error.message} />
              ) : null}
            </div>
          );
        })}
      </div>
    </fieldset>
  );
}
