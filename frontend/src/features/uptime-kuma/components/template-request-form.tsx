import { TemplateBodyForm } from "./template-body-form";
import { requestBodyFields, updateRequestBody, requestEndpointURL } from "../lib/request-body";
import { useMutation } from "@tanstack/react-query";
import { Controller, type UseFormReturn } from "react-hook-form";
import { api, type KumaTemplate } from "@/api";
import { FormField } from "@/App";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { notifyOperationError } from "@/lib/operation-feedback";
import {
  requestProfiles,
  requestProfileLabels,
  requestProfileDetails,
  parseRequestProfile,
  type RequestProfile,
} from "../lib/request-profiles";
import { httpMethods } from "../constants";
import type { TemplateValues } from "../lib/template-schema";
import { MonitorFormSection } from "./monitor-form-section";

const encodingLabels = { json: "JSON", form: "表单（x-www-form-urlencoded）", xml: "XML" };
export function TemplateRequestForm(props: {
  form: UseFormReturn<TemplateValues>;
  item?: KumaTemplate;
  pending: boolean;
}) {
  const form = props.form;
  const errors = form.formState.errors;
  const preset = useMutation({
    mutationKey: ["kuma-template-preset"],
    mutationFn: (next: Exclude<RequestProfile, "">) => {
      const current = requestBodyFields(form.getValues("body")).model;
      const isDefault = Object.values(requestProfileDetails).some((item) => item.model === current);
      return api.kumaTemplatePreset({
        request_profile: next,
        model: current && !isDefault ? current : requestProfileDetails[next].model,
      });
    },
    onSuccess: (value, next) => {
      form.setValue("request_profile", next);
      form.setValue("model", value.model ?? requestProfileDetails[next].model);
      form.setValue("method", "POST");
      form.setValue("body_encoding", "json");
      const current = requestBodyFields(form.getValues("body"));
      const generated = JSON.stringify(JSON.parse(value.body) as unknown, null, 2);
      form.setValue("monitoring.url", requestEndpointURL(form.getValues("monitoring.url"), next), {
        shouldDirty: true,
      });
      form.setValue(
        "body",
        current.messageEditable
          ? updateRequestBody(generated, "message", current.message)
          : generated,
        {
          shouldDirty: true,
        },
      );
      form.setValue("clear_body", false);
      // Only the explicit CLI preset supplies its identification headers.
      if (next === "claude-cli")
        form.setValue("headers", JSON.stringify(JSON.parse(value.headers) as unknown, null, 2), {
          shouldDirty: true,
        });
      form.clearErrors("body");
    },
    onError: (error) => notifyOperationError(error, "请求预设读取失败，请重试"),
  });
  const disabled = props.pending || preset.isPending;
  if (!["http", "keyword"].includes(form.watch("monitoring.type"))) return null;
  return (
    <MonitorFormSection title="请求设置">
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField label="接口模式">
          <Controller
            control={form.control}
            name="request_profile"
            render={({ field }) => (
              <Select
                value={field.value || "custom"}
                disabled={disabled}
                onValueChange={(value) => {
                  const next = parseRequestProfile(value ?? "");
                  if (next) preset.mutate(next);
                  else field.onChange("");
                }}
                itemToStringLabel={(value) => requestProfileLabels[parseRequestProfile(value)]}
              >
                <SelectTrigger aria-label="接口模式">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {requestProfiles.map((value) => (
                    <SelectItem key={value} value={value || "custom"}>
                      {requestProfileLabels[value]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </FormField>
        <FormField label="请求方法">
          <Controller
            control={form.control}
            name="method"
            render={({ field }) => (
              <Select value={field.value} onValueChange={field.onChange} disabled={disabled}>
                <SelectTrigger aria-label="请求方法">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {httpMethods.map((value) => (
                    <SelectItem key={value} value={value}>
                      {value}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </FormField>
        <div className="sm:col-span-2">
          <FormField
            label="请求头（JSON）"
            htmlFor="template-headers"
            error={errors.headers?.message}
          >
            <Textarea
              id="template-headers"
              {...form.register("headers")}
              disabled={disabled}
              aria-invalid={!!errors.headers}
              placeholder="可选，填写需要发送的请求头"
              className="min-h-24 font-mono"
            />
          </FormField>
        </div>
        <FormField label="请求体编码">
          <Controller
            control={form.control}
            name="body_encoding"
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={field.onChange}
                disabled={disabled}
                itemToStringLabel={(value) =>
                  encodingLabels[value as keyof typeof encodingLabels] ?? value
                }
              >
                <SelectTrigger aria-label="请求体编码">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(encodingLabels).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </FormField>
        <TemplateBodyForm form={form} disabled={disabled} />
      </div>
    </MonitorFormSection>
  );
}
