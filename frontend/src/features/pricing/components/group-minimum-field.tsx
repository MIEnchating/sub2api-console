import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, type ReactElement } from "react";
import { Controller, useForm } from "react-hook-form";

import { FormField } from "@/components/form-field";
import { Input } from "@/components/ui/input";
import { groupMinimumSchema, type GroupMinimumForm } from "../lib/group-minimum";

export function GroupMinimumField(props: {
  groupID: string;
  groupName: string;
  value?: string;
  onChange: (groupID: string, value: string) => void;
}): ReactElement {
  const form = useForm<GroupMinimumForm>({
    resolver: zodResolver(groupMinimumSchema),
    defaultValues: { minimum: props.value ?? "" },
    mode: "onChange",
  });
  useEffect(() => {
    const value = props.value ?? "";
    if (form.getValues("minimum") !== value) {
      form.setValue("minimum", value, { shouldValidate: true });
    }
  }, [form, props.value]);
  const inputID = `pricing-group-minimum-${props.groupID}`;
  return (
    <div className="min-w-0 border-t px-3 py-2.5" data-slot="group-minimum-field">
      <Controller
        name="minimum"
        control={form.control}
        render={({ field, fieldState }) => (
          <FormField
            label="最低迁入倍率"
            htmlFor={inputID}
            description="账号成本倍率达到此值才可迁入；留空不限制，低于时按价格选择同一互换组的其他合适分组。"
            error={fieldState.error?.message}
            reserveErrorSpace={false}
          >
            <Input
              {...field}
              id={inputID}
              inputMode="decimal"
              className="min-w-0 tabular-nums"
              aria-label={`分组 ${props.groupName} 最低迁入倍率`}
              aria-invalid={Boolean(fieldState.error)}
              placeholder="不限制"
              onChange={(event) => {
                field.onChange(event);
                props.onChange(props.groupID, event.target.value);
              }}
            />
          </FormField>
        )}
      />
    </div>
  );
}
