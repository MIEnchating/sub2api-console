import { zodResolver } from "@hookform/resolvers/zod";
import { FormField } from "@/components/form-field";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { Save } from "lucide-react";

import type { NewAPIPlatform } from "@/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { sensitiveFieldPlaceholder } from "@/lib/sensitive-field";
import { newAPIPlatformSchema, type NewAPIPlatformValues } from "../lib/schemas";

type Props = {
  open: boolean;
  platform: NewAPIPlatform | null;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: NewAPIPlatformValues) => void;
};

const defaults: NewAPIPlatformValues = {
  name: "",
  base_url: "",
  admin_key: "",
  user_id: "",
};

function Field(props: {
  label: string;
  error?: string;
  help: string;
  helpId: string;
  children: React.ReactNode;
}) {
  return (
    <FormField label={props.label} htmlFor={props.helpId + "-input"} error={props.error}>
      {props.children}
      <p id={props.helpId} className="text-muted-foreground text-xs font-normal">
        {props.help}
      </p>
    </FormField>
  );
}

export function NewAPIPlatformDialog(props: Props) {
  const form = useForm<NewAPIPlatformValues>({
    resolver: zodResolver(newAPIPlatformSchema),
    defaultValues: defaults,
  });

  useEffect(() => {
    form.reset(
      props.platform
        ? {
            name: props.platform.name,
            base_url: props.platform.base_url,
            admin_key: "",
            user_id: props.platform.user_id,
          }
        : defaults,
    );
  }, [form, props.platform, props.open]);

  const address = form.watch("base_url").trim().replace(/\/+$/, "");
  const keyRequired =
    !props.platform?.admin_key_configured ||
    address !== props.platform.base_url.trim().replace(/\/+$/, "");
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!props.pending) props.onOpenChange(open);
      }}
    >
      <DialogContent className="sm:max-w-lg" showCloseButton={!props.pending}>
        <DialogHeader>
          <DialogTitle>
            {props.platform ? "编辑 New API 平台配置" : "添加 New API 平台配置"}
          </DialogTitle>
        </DialogHeader>
        <DialogBody>
          <form
            id="newapi-platform-form"
            className="grid gap-4"
            onSubmit={form.handleSubmit((values) => {
              if (props.pending) return;
              if (keyRequired && !values.admin_key) {
                form.setError("admin_key", {
                  message: "首次接入或更换平台地址时，请填写 Admin Key",
                });
                return;
              }
              props.onSubmit(values);
            })}
          >
            <Field
              label="平台名称"
              helpId="newapi-name-help"
              help="必填 · 用于识别当前 New API 平台。"
              error={form.formState.errors.name?.message}
            >
              <Input
                disabled={props.pending}
                id="newapi-name-help-input"
                aria-describedby="newapi-name-help"
                aria-required={true}
                {...form.register("name")}
                aria-invalid={Boolean(form.formState.errors.name)}
              />
            </Field>
            <Field
              label="平台地址"
              helpId="newapi-base_url-help"
              help="必填 · New API 服务的 HTTP(S) 根地址，不是模型调用接口地址。"
              error={form.formState.errors.base_url?.message}
            >
              <Input
                disabled={props.pending}
                placeholder="https://newapi.example.com"
                id="newapi-base_url-help-input"
                aria-describedby="newapi-base_url-help"
                aria-required={true}
                {...form.register("base_url")}
                aria-invalid={Boolean(form.formState.errors.base_url)}
              />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                label="User ID"
                helpId="newapi-user_id-help"
                help="必填 · 与 Admin Key 对应的 New API 管理员用户 ID。"
                error={form.formState.errors.user_id?.message}
              >
                <Input
                  disabled={props.pending}
                  id="newapi-user_id-help-input"
                  aria-describedby="newapi-user_id-help"
                  aria-required={true}
                  {...form.register("user_id")}
                  aria-invalid={Boolean(form.formState.errors.user_id)}
                />
              </Field>
              <Field
                label="Admin Key"
                helpId="newapi-admin_key-help"
                help="首次接入或更换地址必填；原地址已配置时选填，留空保留原密钥。"
                error={form.formState.errors.admin_key?.message}
              >
                <Input
                  disabled={props.pending}
                  type="password"
                  autoComplete="new-password"
                  placeholder={sensitiveFieldPlaceholder(!keyRequired, "sk-...")}
                  id="newapi-admin_key-help-input"
                  aria-describedby="newapi-admin_key-help"
                  aria-required={keyRequired}
                  {...form.register("admin_key")}
                  aria-invalid={Boolean(form.formState.errors.admin_key)}
                />
              </Field>
            </div>
          </form>
        </DialogBody>
        <DialogFooter>
          <Button
            variant="outline"
            disabled={props.pending}
            onClick={() => props.onOpenChange(false)}
          >
            取消
          </Button>
          <Button form="newapi-platform-form" type="submit" disabled={props.pending}>
            <Save aria-hidden="true" />
            {props.pending ? "正在验证" : "验证并保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
