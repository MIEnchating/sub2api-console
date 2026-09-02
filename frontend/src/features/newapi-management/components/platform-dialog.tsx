import { zodResolver } from "@hookform/resolvers/zod";
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

function Field(props: { label: string; error?: string; children: React.ReactNode }) {
  return (
    <label className="grid gap-1.5 text-sm">
      <span className="font-medium">{props.label}</span>
      {props.children}
      {props.error ? <span className="text-destructive text-xs">{props.error}</span> : null}
    </label>
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

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {props.platform ? "编辑 New API 主平台" : "配置 New API 主平台"}
          </DialogTitle>
        </DialogHeader>
        <DialogBody>
          <form
            id="newapi-platform-form"
            className="grid gap-4"
            onSubmit={form.handleSubmit(props.onSubmit)}
          >
            <Field label="平台名称" error={form.formState.errors.name?.message}>
              <Input
                {...form.register("name")}
                aria-invalid={Boolean(form.formState.errors.name)}
              />
            </Field>
            <Field label="平台地址" error={form.formState.errors.base_url?.message}>
              <Input
                placeholder="https://newapi.example.com"
                {...form.register("base_url")}
                aria-invalid={Boolean(form.formState.errors.base_url)}
              />
            </Field>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="User ID" error={form.formState.errors.user_id?.message}>
                <Input
                  {...form.register("user_id")}
                  aria-invalid={Boolean(form.formState.errors.user_id)}
                />
              </Field>
              <Field label="Admin Key" error={form.formState.errors.admin_key?.message}>
                <Input
                  type="password"
                  autoComplete="new-password"
                  placeholder={sensitiveFieldPlaceholder(Boolean(props.platform), "sk-...")}
                  {...form.register("admin_key")}
                  aria-invalid={Boolean(form.formState.errors.admin_key)}
                />
              </Field>
            </div>
          </form>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
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
