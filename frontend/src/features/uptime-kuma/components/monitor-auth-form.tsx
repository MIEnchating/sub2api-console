import { Controller, type UseFormReturn } from "react-hook-form";
import type { KumaMonitor } from "@/api";
import { FormField } from "@/App";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { requestAuthLabels } from "../constants";
import type { MonitorValues } from "../lib/schemas";
import { MonitorFormSection } from "./monitor-form-section";
import { cn } from "@/lib/utils";

export function MonitorAuthForm(props: {
  form: UseFormReturn<MonitorValues>;
  monitor: KumaMonitor | null;
  pending: boolean;
}) {
  const form = props.form;
  const usingTemplate = !!form.watch("template_id");
  const method = form.watch("options.auth_method") ?? "none";
  const errors = form.formState.errors.options;
  const retained = !!form.watch("template_retain");
  const canPreserve = (!usingTemplate || retained) && !!props.monitor;
  const usernameLabel = canPreserve ? "鉴权用户名（留空保留）" : "鉴权用户名";
  const passwordLabel = canPreserve ? "鉴权密码 / Token（留空保留）" : "鉴权密码 / Token";
  const changeMethod = (value: string | null): void => {
    if (!value) return;
    form.setValue("template_auth_override", usingTemplate && !retained);
    form.setValue("options.auth_method", value);
    form.setValue("options.auth_username", "");
    form.setValue("options.auth_password", "");
    form.setValue("options.clear_auth", false);
    form.clearErrors(["options.auth_username", "options.auth_password"]);
  };
  return (
    <MonitorFormSection title="鉴权设置">
      <div className={cn("grid gap-3 sm:grid-cols-2", method === "basic" && "lg:grid-cols-3")}>
        <FormField label="HTTP 鉴权方式" htmlFor="kuma-auth-method">
          <Select
            value={method}
            onValueChange={changeMethod}
            disabled={props.pending}
            itemToStringLabel={(value) => requestAuthLabels[value ?? "none"] ?? "保留现有鉴权"}
          >
            <SelectTrigger id="kuma-auth-method">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(requestAuthLabels).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
              {!usingTemplate && !["none", "basic", "bearer"].includes(method) && (
                <SelectItem value={method}>保留现有鉴权</SelectItem>
              )}
            </SelectContent>
          </Select>
        </FormField>
        {method === "basic" && (
          <FormField
            label={usernameLabel}
            htmlFor="kuma-auth-user"
            error={errors?.auth_username?.message}
          >
            <Input
              id="kuma-auth-user"
              {...form.register("options.auth_username")}
              autoComplete="off"
              aria-invalid={!!errors?.auth_username}
            />
          </FormField>
        )}
        {["basic", "bearer"].includes(method) && (
          <FormField
            label={passwordLabel}
            htmlFor="kuma-auth-pass"
            error={errors?.auth_password?.message}
          >
            <Input
              id="kuma-auth-pass"
              type="password"
              {...form.register("options.auth_password")}
              autoComplete="new-password"
              aria-invalid={!!errors?.auth_password}
              placeholder={
                canPreserve && props.monitor?.options?.auth_configured ? "已配置，留空保留" : ""
              }
            />
          </FormField>
        )}
      </div>
      {canPreserve && (
        <label className="flex items-center gap-2 text-sm">
          <Controller
            control={form.control}
            name="options.clear_auth"
            render={({ field }) => (
              <Checkbox
                checked={!!field.value}
                onCheckedChange={field.onChange}
                disabled={props.pending}
              />
            )}
          />
          清空已有鉴权凭据
        </label>
      )}
    </MonitorFormSection>
  );
}
