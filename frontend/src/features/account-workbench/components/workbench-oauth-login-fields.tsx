import type { ReactElement } from "react";
import { FormProvider, type UseFormReturn } from "react-hook-form";
import { FormField } from "@/components/form-field";
import { Input } from "@/components/ui/input";
import type { OAuthLoginValues } from "../lib/oauth-login-schema";
import { WorkbenchOAuthMailboxFields } from "./workbench-oauth-mailbox";
import { WorkbenchOAuthSMSFields } from "./workbench-oauth-sms";

const fields = [
  { name: "email", label: "登录邮箱", suffix: "email", type: "text" },
  { name: "password", label: "登录密码", suffix: "password", type: "password" },
  { name: "totp_secret", label: "TOTP 密钥", suffix: "totp", type: "password" },
  { name: "workspace_id", label: "工作区 ID", suffix: "workspace", type: "text" },
  { name: "proxy_url", label: "登录代理", suffix: "proxy", type: "password" },
] as const;

export function WorkbenchOAuthLoginFields(props: {
  form: UseFormReturn<OAuthLoginValues>;
  identityReadOnly?: boolean;
}): ReactElement {
  return (
    <>
      {fields.map((field) => (
        <FormField
          key={field.name}
          label={field.label}
          htmlFor={`oauth-login-${field.suffix}`}
          error={props.form.formState.errors[field.name]?.message}
        >
          <Input
            id={`oauth-login-${field.suffix}`}
            type={field.type}
            autoComplete="off"
            readOnly={
              props.identityReadOnly && (field.name === "email" || field.name === "workspace_id")
            }
            aria-invalid={!!props.form.formState.errors[field.name]}
            {...props.form.register(field.name)}
          />
        </FormField>
      ))}
      <WorkbenchOAuthMailboxFields form={props.form} />
      <FormProvider {...props.form}>
        <WorkbenchOAuthSMSFields />
      </FormProvider>
    </>
  );
}
