import { useId, type ReactElement } from "react";
import type { UseFormReturn } from "react-hook-form";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { FieldError } from "@/components/field-error";
import { cn } from "@/lib/utils";
import { detectionStageOptions } from "../constants";
import type { DetectionTaskForm } from "../lib/detection-task-schema";
import { detectionTaskStages } from "../lib/detection-task-stages";
import { PrecheckQuestionSelector } from "./precheck-question-selector";

export function DetectionTaskStages(props: {
  form: UseFormReturn<DetectionTaskForm>;
  pending: boolean;
}): ReactElement {
  const id = useId();
  const form = props.form;
  const value = form.watch();
  const errors = form.formState.errors;
  const stages = detectionTaskStages(value);
  return (
    <fieldset disabled={props.pending} className="min-w-0 space-y-3 border-t pt-4">
      <legend className="px-1 text-sm font-medium">检测内容</legend>
      <p id={`${id}-hint`} className="text-xs text-muted-foreground">
        可多选，至少选择一项。
      </p>
      <div className="grid min-w-0 gap-3 sm:grid-cols-3">
        {detectionStageOptions.map((option) => (
          <label
            key={option.value}
            className={cn(
              "flex min-w-0 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors has-focus-visible:ring-2 has-focus-visible:ring-ring",
              value[option.value] ? "border-primary/60 bg-primary/5" : "bg-card hover:bg-muted/40",
              props.pending && "cursor-not-allowed opacity-60",
            )}
          >
            <Checkbox
              aria-labelledby={`${id}-${option.value}-label`}
              aria-invalid={!!errors.animation}
              aria-describedby={`${id}-hint ${id}-${option.value}${errors.animation ? ` ${id}-error` : ""}`}
              checked={value[option.value]}
              disabled={props.pending}
              onCheckedChange={(checked) => {
                form.setValue(option.value, checked, { shouldDirty: true });
                void form.trigger(["animation", option.value]);
              }}
              className="mt-0.5"
            />
            <span className="min-w-0 space-y-1">
              <span id={`${id}-${option.value}-label`} className="block text-sm font-medium">
                {option.label}
              </span>
              <span
                id={`${id}-${option.value}`}
                className="block text-xs text-muted-foreground wrap-anywhere"
              >
                {option.description}
              </span>
            </span>
          </label>
        ))}
      </div>
      <FieldError id={`${id}-error`} message={errors.animation?.message} />
      {(value.precheck || value.terminal) && (
        <div className="grid min-w-0 items-start gap-4 rounded-lg bg-muted/30 p-3 sm:grid-cols-2">
          {value.precheck && (
            <div className="min-w-0 space-y-2">
              <p className="text-sm font-medium">前置检测题目</p>
              <PrecheckQuestionSelector
                value={value.precheck_questions}
                onChange={(questions) =>
                  form.setValue("precheck_questions", questions, { shouldValidate: true })
                }
                disabled={props.pending}
              />
              <FieldError message={errors.precheck_questions?.message} />
            </div>
          )}
          {value.terminal && (
            <div className="min-w-0 space-y-2">
              <label className="block space-y-1 text-sm">
                终端检测轮数
                <Input
                  type="number"
                  min={1}
                  max={20}
                  {...form.register("terminal_rounds", { valueAsNumber: true })}
                  disabled={props.pending}
                  aria-invalid={!!errors.terminal_rounds}
                />
              </label>
              <FieldError message={errors.terminal_rounds?.message} />
              <p className="text-xs text-muted-foreground">每轮独立请求并保存结果，最多 20 轮。</p>
            </div>
          )}
        </div>
      )}
      <p className="text-xs text-muted-foreground wrap-anywhere">
        {stages.length > 0 ? `执行顺序：${stages.join(" → ")}。` : "请选择要执行的检测。"}
        各阶段分别保存结果，不修改健康分或调度。
      </p>
    </fieldset>
  );
}
