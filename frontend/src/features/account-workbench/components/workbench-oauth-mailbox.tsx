import type { ReactElement } from "react";
import { Controller, type UseFormReturn } from "react-hook-form";
import { FormField } from "@/components/form-field";
import { JsonEditorField } from "@/components/json-editor/form-field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { OAuthLoginValues } from "../lib/oauth-login-schema";

const mailboxOptions = [
  { value: "none", label: "人工填写" },
  { value: "http", label: "HTTP 邮箱" },
  { value: "microsoft", label: "Microsoft 邮箱" },
] as const;

export function WorkbenchOAuthMailboxFields(props: {
  form: UseFormReturn<OAuthLoginValues>;
}): ReactElement {
  const kind = props.form.watch("mailbox_kind");
  return (
    <div className="grid min-w-0 gap-3 border-t pt-3">
      <FormField label="邮箱验证码">
        <Controller
          control={props.form.control}
          name="mailbox_kind"
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger aria-label="邮箱验证码">
                <SelectValue>
                  {mailboxOptions.find((item) => item.value === field.value)?.label}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {mailboxOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
      </FormField>
      {kind === "http" && (
        <>
          <FormField
            label="邮箱接口地址"
            htmlFor="oauth-mailbox-url"
            error={props.form.formState.errors.url?.message}
          >
            <Input
              id="oauth-mailbox-url"
              autoComplete="off"
              aria-invalid={!!props.form.formState.errors.url}
              {...props.form.register("url")}
            />
          </FormField>
          <FormField label="请求方法">
            <Controller
              control={props.form.control}
              name="method"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger aria-label="请求方法">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="GET">GET</SelectItem>
                    <SelectItem value="POST">POST</SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>
          <FormField label="请求头 JSON" error={props.form.formState.errors.headers?.message}>
            <JsonEditorField
              control={props.form.control}
              name="headers"
              aria-label="邮箱请求头 JSON"
              className="h-40"
            />
          </FormField>
          {props.form.watch("method") === "POST" && (
            <FormField label="请求体 JSON" error={props.form.formState.errors.body?.message}>
              <JsonEditorField
                control={props.form.control}
                name="body"
                aria-label="邮箱请求体 JSON"
                className="h-40"
              />
            </FormField>
          )}
        </>
      )}
      {kind === "microsoft" && (
        <>
          <FormField
            label="收信邮箱"
            htmlFor="oauth-mailbox-email"
            error={props.form.formState.errors.mailbox_email?.message}
          >
            <Input
              id="oauth-mailbox-email"
              autoComplete="off"
              placeholder="留空使用登录邮箱"
              aria-invalid={!!props.form.formState.errors.mailbox_email}
              {...props.form.register("mailbox_email")}
            />
          </FormField>
          <FormField
            label="Microsoft 客户端 ID"
            htmlFor="oauth-mailbox-client"
            error={props.form.formState.errors.client_id?.message}
          >
            <Input
              id="oauth-mailbox-client"
              autoComplete="off"
              aria-invalid={!!props.form.formState.errors.client_id}
              {...props.form.register("client_id")}
            />
          </FormField>
          <FormField
            label="Microsoft Refresh Token"
            htmlFor="oauth-mailbox-token"
            error={props.form.formState.errors.refresh_token?.message}
          >
            <Input
              id="oauth-mailbox-token"
              type="password"
              autoComplete="off"
              aria-invalid={!!props.form.formState.errors.refresh_token}
              {...props.form.register("refresh_token")}
            />
          </FormField>
        </>
      )}
    </div>
  );
}
