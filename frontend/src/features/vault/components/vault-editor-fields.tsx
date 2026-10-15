import { useId, type ReactElement, type ReactNode } from "react";
import type { UseFormReturn } from "react-hook-form";
import { Eraser, Undo2 } from "lucide-react";
import type { VaultEntryIndex } from "@/api";
import { FieldError } from "@/components/field-error";
import { FieldLabel } from "@/components/field-help-tooltip";
import { JsonEditorField } from "@/components/json-editor/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Textarea } from "@/components/ui/textarea";
import { sensitiveFieldPlaceholder } from "@/lib/sensitive-field";
import type { MutableVaultField, VaultForm } from "../lib/vault-schema";

function Field(props: {
  label: string;
  id: string;
  hint?: string;
  error?: string;
  children: ReactNode;
}): ReactElement {
  return (
    <div className="grid min-w-0 content-start gap-1.5 text-sm">
      <FieldLabel label={props.label} htmlFor={props.id} description={props.hint} />
      {props.children}
      <FieldError id={`${props.id}-error`} message={props.error} />
    </div>
  );
}

export function VaultEditorFields(props: {
  form: UseFormReturn<VaultForm>;
  editing: VaultEntryIndex | null;
  cleared: Set<MutableVaultField>;
  disabled: boolean;
  onChange: (field: MutableVaultField) => void;
  onToggleClear: (field: MutableVaultField) => void;
}): ReactElement {
  const id = useId();
  const errors = props.form.formState.errors;
  return (
    <fieldset disabled={props.disabled} className="grid min-w-0 gap-4 disabled:opacity-70">
      <fieldset className="grid min-w-0 gap-3 rounded-lg border bg-muted/15 p-3">
        <legend className="px-1 text-sm font-medium">基本信息</legend>
        <Field label="凭据名称" id={`${id}-entry`} error={errors.entry?.message}>
          <Input
            id={`${id}-entry`}
            {...props.form.register("entry", { onChange: () => props.form.clearErrors("entry") })}
            aria-invalid={Boolean(errors.entry)}
            aria-describedby={errors.entry ? `${id}-entry-error` : undefined}
            placeholder="例如：主站管理员"
            autoComplete="off"
          />
          <p className="text-xs text-muted-foreground">
            用于识别凭据；改名会保留已保存的账号密码及关联设置。
          </p>
        </Field>
        <Field
          label="匹配 Host"
          id={`${id}-hosts`}
          hint="多个 Host 使用逗号或换行分隔；留空时匹配全部 Host。"
        >
          <Textarea
            id={`${id}-hosts`}
            {...props.form.register("hosts", { onChange: () => props.onChange("hosts") })}
            className="min-h-16 resize-y font-mono text-xs"
            rows={2}
            placeholder="api.example.com"
            spellCheck={false}
          />
        </Field>
      </fieldset>
      <fieldset className="grid min-w-0 gap-3 rounded-lg border bg-muted/15 p-3 sm:grid-cols-2">
        <legend className="px-1 text-sm font-medium">登录凭据</legend>
        {(["username", "password"] as const).map((field) => {
          const label = field === "username" ? "用户名" : "密码";
          const configured =
            field === "username" ? props.editing?.has_username : props.editing?.has_password;
          const cleared = props.cleared.has(field);
          return (
            <Field key={field} label={label} id={`${id}-${field}`}>
              <div className="flex min-w-0 items-center gap-2">
                <Input
                  id={`${id}-${field}`}
                  type={field === "password" ? "password" : "text"}
                  autoComplete={field === "password" ? "new-password" : "username"}
                  className="min-w-0 flex-1"
                  {...props.form.register(field, { onChange: () => props.onChange(field) })}
                  placeholder={
                    cleared
                      ? "保存后清除"
                      : sensitiveFieldPlaceholder(Boolean(configured), `输入${label}`)
                  }
                />
                {configured && (
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label={`${cleared ? "撤销清除" : "清除"}${label}`}
                          onClick={() => props.onToggleClear(field)}
                        />
                      }
                    >
                      {cleared ? <Undo2 aria-hidden="true" /> : <Eraser aria-hidden="true" />}
                    </TooltipTrigger>
                    <TooltipContent>{`${cleared ? "撤销清除" : "清除"}${label}`}</TooltipContent>
                  </Tooltip>
                )}
              </div>
              {cleared && <p className="text-xs text-warning">保存后将移除此{label}</p>}
            </Field>
          );
        })}
      </fieldset>
      <fieldset className="grid min-w-0 gap-3 rounded-lg border bg-muted/15 p-3">
        <legend className="px-1 text-sm font-medium">
          额外请求头 <span className="font-normal text-muted-foreground">· 可选</span>
        </legend>
        <Field
          label="Headers JSON"
          id={`${id}-headers`}
          error={errors.headers?.message}
          hint="仅在登录接口需要额外请求头时填写；已配置的内容留空则不修改。"
        >
          {Boolean(props.editing?.header_names.length) && (
            <p className="text-xs text-muted-foreground [overflow-wrap:anywhere]">
              已配置：{props.editing?.header_names.join("、")}
            </p>
          )}
          <JsonEditorField
            control={props.form.control}
            name="headers"
            id={`${id}-headers`}
            aria-label="Headers JSON"
            disabled={props.disabled}
            className="h-36"
            onValueChange={() => props.onChange("headers")}
            placeholder={
              props.cleared.has("headers")
                ? "保存后清除"
                : sensitiveFieldPlaceholder(
                    Boolean(props.editing?.header_names.length),
                    '{"X-Custom-Header":"value"}',
                  )
            }
          />
          {Boolean(props.editing?.header_names.length) && (
            <Button
              type="button"
              variant="ghost"
              className="w-fit"
              onClick={() => props.onToggleClear("headers")}
            >
              {props.cleared.has("headers") ? (
                <Undo2 aria-hidden="true" />
              ) : (
                <Eraser aria-hidden="true" />
              )}
              {props.cleared.has("headers") ? "撤销清除 Headers" : "清除 Headers"}
            </Button>
          )}
          {props.cleared.has("headers") && (
            <p className="text-xs text-warning">保存后将移除全部请求头</p>
          )}
        </Field>
      </fieldset>
    </fieldset>
  );
}
