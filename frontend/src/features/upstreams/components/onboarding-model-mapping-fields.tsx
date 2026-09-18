import { Plus, Trash2 } from "lucide-react";
import { useId, type ReactElement } from "react";
import { Controller, useFieldArray, type UseFormReturn } from "react-hook-form";
import { FieldError } from "@/components/field-error";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { OnboardingConfirmationForm } from "../lib/onboarding-model-mapping";

export function OnboardingModelMappingFields(props: {
  form: UseFormReturn<OnboardingConfirmationForm>;
  index: number;
  disabled: boolean;
  models?: string[];
  modelsPending: boolean;
}): ReactElement {
  const id = useId();
  const models = props.models ?? [];
  const name = `accounts.${props.index}.mapping` as const;
  const rows = useFieldArray({ control: props.form.control, name });
  const errors = props.form.formState.errors.accounts?.[props.index]?.mapping;
  return (
    <fieldset disabled={props.disabled} className="grid min-w-0 gap-2">
      <legend className="sr-only">模型映射（可选）</legend>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span aria-hidden="true" className="text-sm font-medium">
          模型映射 <span className="text-muted-foreground font-normal">（可选）</span>
        </span>
        <Button
          type="button"
          variant="outline"
          disabled={rows.fields.length >= 100 || props.disabled}
          onClick={() => rows.append({ source: "", target: "" })}
        >
          <Plus aria-hidden="true" />
          添加模型映射
        </Button>
      </div>
      <p className="text-muted-foreground text-xs">
        选择上游模型后自动回填，请求模型可改名；留空保留默认映射。
      </p>
      {rows.fields.length > 0 ? (
        <div
          aria-hidden="true"
          className="text-muted-foreground hidden grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2rem] gap-2 text-xs sm:grid"
        >
          <span>请求模型</span>
          <span>上游模型</span>
          <span />
        </div>
      ) : null}
      {rows.fields.map((row, index) => {
        const sourceError = errors?.[index]?.source?.message;
        const targetError = errors?.[index]?.target?.message;
        return (
          <div
            key={row.id}
            role="group"
            aria-label={`第 ${index + 1} 条模型映射`}
            className="grid min-w-0 grid-cols-[minmax(0,1fr)_2rem] items-start gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2rem]"
          >
            <div className="col-start-1 grid min-w-0 gap-1.5">
              <label htmlFor={`${id}-${index}-source`} className="text-sm sm:sr-only">
                请求模型
              </label>
              <Input
                id={`${id}-${index}-source`}
                {...props.form.register(`${name}.${index}.source`)}
                aria-invalid={Boolean(sourceError)}
                aria-describedby={sourceError ? `${id}-${index}-source-error` : undefined}
                placeholder="例如 gpt-5"
              />
              <FieldError id={`${id}-${index}-source-error`} message={sourceError} />
            </div>
            <div className="col-start-1 grid min-w-0 gap-1.5 sm:col-start-2 sm:row-start-1">
              <label htmlFor={`${id}-${index}-target`} className="text-sm sm:sr-only">
                上游模型
              </label>
              <Controller
                control={props.form.control}
                name={`${name}.${index}.target`}
                render={(controller) => (
                  <Select
                    value={controller.field.value || null}
                    disabled={props.disabled || props.modelsPending || models.length === 0}
                    onValueChange={(value) => {
                      if (value === null) return;
                      const sourceName = `${name}.${index}.source` as const;
                      const currentSource = props.form.getValues(sourceName);
                      if (!currentSource || currentSource === controller.field.value) {
                        props.form.setValue(sourceName, value, {
                          shouldDirty: true,
                          shouldValidate: true,
                        });
                      }
                      controller.field.onChange(value);
                    }}
                  >
                    <SelectTrigger
                      id={`${id}-${index}-target`}
                      ref={controller.field.ref}
                      onBlur={controller.field.onBlur}
                      aria-invalid={Boolean(targetError)}
                      aria-describedby={targetError ? `${id}-${index}-target-error` : undefined}
                    >
                      <SelectValue placeholder={models.length ? "选择上游模型" : "请先获取模型"} />
                    </SelectTrigger>
                    <SelectContent>
                      {models.map((model) => (
                        <SelectItem key={model} value={model}>
                          {model}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              <FieldError id={`${id}-${index}-target-error`} message={targetError} />
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`删除第 ${index + 1} 条模型映射`}
              className="col-start-2 row-span-2 row-start-1 mt-6 sm:col-start-3 sm:row-span-1 sm:mt-0"
              onClick={() => rows.remove(index)}
            >
              <Trash2 aria-hidden="true" />
            </Button>
          </div>
        );
      })}
    </fieldset>
  );
}
