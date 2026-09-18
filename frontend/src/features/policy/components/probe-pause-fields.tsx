import { zodResolver } from "@hookform/resolvers/zod";
import { useId, type ReactElement } from "react";
import { Controller, useForm } from "react-hook-form";

import { FormField } from "@/components/form-field";
import { Input } from "@/components/ui/input";
import {
  defaultProbePauseWindow,
  probePauseWindowSchema,
  type ProbePauseWindowValues,
} from "../lib/probe-pause-schema";
import { PolicySwitchRow } from "./policy-switch-row";

export function ProbePauseFields(props: {
  value: unknown;
  onChange: (value: ProbePauseWindowValues) => void;
}): ReactElement {
  const id = useId();
  const values = {
    ...defaultProbePauseWindow,
    ...(props.value && typeof props.value === "object" ? props.value : {}),
  } as ProbePauseWindowValues;
  const form = useForm<ProbePauseWindowValues>({
    resolver: zodResolver(probePauseWindowSchema),
    mode: "onChange",
    values,
  });

  function update(key: keyof ProbePauseWindowValues, value: string | boolean): void {
    form.setValue(key, value, { shouldDirty: true, shouldValidate: true });
    void form.trigger();
    props.onChange({ ...form.getValues(), [key]: value });
  }

  return (
    <fieldset className="col-span-full grid min-w-0 gap-4 border-t pt-4">
      <legend className="sr-only">自动探活暂停时段</legend>
      <Controller
        control={form.control}
        name="enabled"
        render={({ field }) => (
          <PolicySwitchRow
            label="启用每日探活暂停时段"
            description=""
            checked={field.value}
            onCheckedChange={(enabled) => update("enabled", enabled)}
          />
        )}
      />
      <div className="grid min-w-0 grid-cols-1 gap-4 sm:grid-cols-3">
        <FormField
          label="暂停开始时间"
          htmlFor={`${id}-start`}
          error={form.formState.errors.start?.message}
        >
          <Input
            id={`${id}-start`}
            type="time"
            disabled={!values.enabled}
            aria-invalid={Boolean(form.formState.errors.start)}
            {...form.register("start")}
            onChange={(event) => update("start", event.target.value)}
          />
        </FormField>
        <FormField
          label="暂停结束时间"
          htmlFor={`${id}-end`}
          error={form.formState.errors.end?.message}
        >
          <Input
            id={`${id}-end`}
            type="time"
            disabled={!values.enabled}
            aria-invalid={Boolean(form.formState.errors.end)}
            {...form.register("end")}
            onChange={(event) => update("end", event.target.value)}
          />
        </FormField>
        <FormField
          label="暂停时区"
          htmlFor={`${id}-timezone`}
          error={form.formState.errors.timezone?.message}
        >
          <Input
            id={`${id}-timezone`}
            disabled={!values.enabled}
            aria-invalid={Boolean(form.formState.errors.timezone)}
            {...form.register("timezone")}
            onChange={(event) => update("timezone", event.target.value)}
          />
        </FormField>
      </div>
    </fieldset>
  );
}
