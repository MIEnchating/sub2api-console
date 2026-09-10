import { Controller, type UseFormReturn } from "react-hook-form";
import { FormField } from "@/App";
import { Input } from "@/components/ui/input";
import type { TemplateValues } from "../lib/template-schema";
import { requestEndpointURL } from "../lib/request-body";

export function TemplateAddressField(props: {
  form: UseFormReturn<TemplateValues>;
  pending: boolean;
  configured: boolean;
}) {
  const error = props.form.formState.errors.monitoring?.url;
  return (
    <FormField label="监控地址" htmlFor="template-url" error={error?.message}>
      <Controller
        control={props.form.control}
        name="monitoring.url"
        render={({ field }) => (
          <Input
            id="template-url"
            {...field}
            disabled={props.pending}
            aria-invalid={!!error}
            onBlur={() => {
              field.onBlur();
              field.onChange(
                requestEndpointURL(field.value.trim(), props.form.getValues("request_profile")),
              );
            }}
            placeholder={props.configured ? "已配置，留空保留" : "https://你的服务地址"}
          />
        )}
      />
    </FormField>
  );
}
