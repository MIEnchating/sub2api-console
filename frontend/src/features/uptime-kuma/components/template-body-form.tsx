import { useId, useState } from "react";
import type { UseFormReturn } from "react-hook-form";
import { ChevronDown, ChevronUp } from "lucide-react";
import { FormField } from "@/App";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import type { TemplateValues } from "../lib/template-schema";
import { requestBodyFields, updateRequestBody } from "../lib/request-body";

export function TemplateBodyForm(props: {
  form: UseFormReturn<TemplateValues>;
  disabled: boolean;
}) {
  const form = props.form;
  const body = form.watch("body");
  const profile = form.watch("request_profile");
  const encoding = form.watch("body_encoding");
  const fields = requestBodyFields(body);
  const [expanded, setExpanded] = useState(false);
  const id = useId();
  const error = form.formState.errors.body;
  const visible = expanded || !!error;
  const update = (field: "model" | "message", value: string): void => {
    form.setValue("body", updateRequestBody(body, field, value), { shouldDirty: true });
    if (field === "model") form.setValue("model", value, { shouldDirty: true });
  };
  return (
    <>
      {!!profile && encoding === "json" && (
        <>
          <FormField
            label="请求模型"
            htmlFor={`${id}-model`}
            error={form.formState.errors.model?.message}
          >
            <Input
              id={`${id}-model`}
              value={fields.model}
              onChange={(event) => update("model", event.target.value)}
              disabled={props.disabled || !fields.editable}
              aria-invalid={!!form.formState.errors.model}
            />
          </FormField>
          <div className="sm:col-span-2">
            <FormField label="发送消息" htmlFor={`${id}-message`}>
              <Textarea
                id={`${id}-message`}
                value={fields.message}
                onChange={(event) => update("message", event.target.value)}
                disabled={props.disabled || !fields.messageEditable}
                className="min-h-24"
              />
            </FormField>
          </div>
        </>
      )}
      <div className="sm:col-span-2">
        <Button
          type="button"
          variant="outline"
          aria-expanded={visible}
          aria-controls={`${id}-body`}
          onClick={() => setExpanded(!visible)}
        >
          {visible ? <ChevronUp aria-hidden="true" /> : <ChevronDown aria-hidden="true" />}
          {visible ? "收起请求体" : "查看请求体"}
        </Button>
        {visible && (
          <div id={`${id}-body`} className="mt-3">
            <FormField label="请求体" htmlFor="template-body" error={error?.message}>
              <Textarea
                id="template-body"
                {...form.register("body")}
                disabled={props.disabled}
                aria-invalid={!!error}
                onChange={(event) => {
                  form.setValue("body", event.target.value, { shouldDirty: true });
                  const next = requestBodyFields(event.target.value);
                  if (next.editable) form.setValue("model", next.model);
                }}
                placeholder="填写请求体"
                className="min-h-48 font-mono"
              />
            </FormField>
          </div>
        )}
      </div>
    </>
  );
}
