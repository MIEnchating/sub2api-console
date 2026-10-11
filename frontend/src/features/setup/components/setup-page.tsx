import { useId, useMemo, useRef } from "react";
import { useMutation } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { api, type SetupStatus } from "@/api";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { notifyOperationError } from "@/lib/operation-feedback";
import { createSetupSchema, type SetupForm } from "../lib/setup-schema";

export function SetupPage(props: {
  status?: SetupStatus;
  onComplete: (status: SetupStatus) => void;
}) {
  const fieldID = useId();
  const setupTokenRequired = props.status?.setup_token_required === true;
  const schema = useMemo(
    () => createSetupSchema(setupTokenRequired, props.status?.target_configured === true),
    [setupTokenRequired, props.status?.target_configured],
  );
  const form = useForm<SetupForm>({
    resolver: zodResolver(schema),
    defaultValues: {
      local_export_only: false,
      username: "",
      password: "",
      confirm_password: "",
      admin_base_url: "",
      admin_key: "",
      setup_token: "",
    },
  });
  const pending = useRef<Parameters<typeof api.initialize> | null>(null);
  const submitting = useRef(false);
  function complete(status: SetupStatus): void {
    form.reset();
    props.onComplete(status);
  }
  const initialize = useMutation({
    networkMode: "always",
    retry: false,
    gcTime: 0,
    mutationFn: async () => {
      const input = pending.current;
      pending.current = null;
      if (!input) throw new Error("提交内容已失效，请重新填写后初始化");
      const status = await api.initialize(...input);
      if (status?.initialized !== true) throw new Error("服务端未确认初始化完成，请重试");
      return status;
    },
    onSuccess: complete,
    onError: async (reason: unknown) => {
      try {
        const status = await api.setupStatus();
        if (status.initialized === true) {
          toast.info("控制台已初始化，正在确认登录状态");
          complete(status);
          return;
        }
      } catch {
        // Preserve the original write error and allow an explicit retry.
      }
      notifyOperationError(reason, "初始化失败");
    },
    onSettled: () => {
      submitting.current = false;
      pending.current = null;
      initialize.reset();
    },
  });
  const submit = form.handleSubmit((values) => {
    if (submitting.current) return;
    submitting.current = true;
    pending.current = [
      {
        username: values.username,
        password: values.password,
        local_export_only: values.local_export_only,
        admin_base_url:
          props.status?.target_configured || values.local_export_only ? "" : values.admin_base_url,
        admin_key:
          props.status?.target_configured || values.local_export_only ? "" : values.admin_key,
      },
      setupTokenRequired ? values.setup_token : undefined,
    ];
    initialize.mutate();
  });
  return (
    <div className="bg-background text-foreground grid min-h-svh place-items-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="flex items-start gap-3">
            <img
              src="/console-mark.svg"
              width={36}
              height={36}
              alt=""
              className="size-9 shrink-0"
            />
            <div>
              <div className="text-muted-foreground text-[11px] font-medium tracking-wider uppercase">
                FIRST RUN / SETUP
              </div>
              <CardTitle className="mt-1">初始化控制台</CardTitle>
              <p className="text-muted-foreground mt-2 text-sm">
                {props.status?.target_configured
                  ? "管理目标已存在，只需设置控制台登录账号。"
                  : "设置登录账号，并连接 Sub2API 管理端。"}
              </p>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} aria-busy={initialize.isPending}>
            <fieldset disabled={initialize.isPending} className="grid gap-4">
              <FormField
                label="控制台账号"
                htmlFor={`${fieldID}-username`}
                error={form.formState.errors.username?.message}
              >
                <Input
                  id={`${fieldID}-username`}
                  autoComplete="username"
                  aria-invalid={Boolean(form.formState.errors.username)}
                  {...form.register("username")}
                  placeholder="例如 operator"
                />
              </FormField>
              <FormField
                label="控制台密码"
                htmlFor={`${fieldID}-password`}
                error={form.formState.errors.password?.message}
              >
                <Input
                  id={`${fieldID}-password`}
                  type="password"
                  autoComplete="new-password"
                  aria-invalid={Boolean(form.formState.errors.password)}
                  {...form.register("password")}
                  placeholder="至少 10 个字符"
                />
              </FormField>
              <FormField
                label="确认控制台密码"
                htmlFor={`${fieldID}-confirm-password`}
                error={form.formState.errors.confirm_password?.message}
              >
                <Input
                  id={`${fieldID}-confirm-password`}
                  type="password"
                  autoComplete="new-password"
                  aria-invalid={Boolean(form.formState.errors.confirm_password)}
                  {...form.register("confirm_password")}
                />
              </FormField>
              {setupTokenRequired ? (
                <FormField
                  label="初始化令牌"
                  htmlFor={`${fieldID}-setup-token`}
                  error={form.formState.errors.setup_token?.message}
                >
                  <Input
                    id={`${fieldID}-setup-token`}
                    type="password"
                    autoComplete="off"
                    required
                    aria-label="初始化令牌"
                    aria-invalid={Boolean(form.formState.errors.setup_token)}
                    {...form.register("setup_token")}
                  />
                </FormField>
              ) : null}
              {!props.status?.target_configured && (
                <>
                  <label className="flex items-start gap-2 text-sm">
                    <Checkbox
                      checked={form.watch("local_export_only")}
                      disabled={initialize.isPending}
                      onCheckedChange={(value) => {
                        const local = value === true;
                        form.setValue("local_export_only", local);
                        if (local) {
                          form.setValue("admin_base_url", "");
                          form.setValue("admin_key", "");
                          form.clearErrors(["admin_base_url", "admin_key"]);
                        }
                      }}
                    />
                    仅使用本地账号工作台（不配置线上管理目标）
                  </label>
                  {!form.watch("local_export_only") && (
                    <>
                      <FormField
                        label="Admin Base URL"
                        htmlFor={`${fieldID}-admin-base-url`}
                        error={form.formState.errors.admin_base_url?.message}
                      >
                        <Input
                          id={`${fieldID}-admin-base-url`}
                          type="url"
                          aria-invalid={Boolean(form.formState.errors.admin_base_url)}
                          {...form.register("admin_base_url")}
                          placeholder="https://sub2api.example.com"
                        />
                      </FormField>
                      <FormField
                        label="Admin Key"
                        htmlFor={`${fieldID}-admin-key`}
                        error={form.formState.errors.admin_key?.message}
                      >
                        <Input
                          id={`${fieldID}-admin-key`}
                          type="password"
                          autoComplete="off"
                          aria-invalid={Boolean(form.formState.errors.admin_key)}
                          {...form.register("admin_key")}
                          placeholder="只提交到后端，不会回显"
                        />
                      </FormField>
                    </>
                  )}
                </>
              )}
              <Button type="submit" disabled={initialize.isPending}>
                <ShieldCheck size={16} />
                {initialize.isPending ? "正在初始化…" : "完成初始化"}
              </Button>
            </fieldset>
          </form>
          <div className="border-border text-muted-foreground mt-5 flex items-start gap-2 border-t pt-4 text-xs leading-relaxed">
            <ShieldCheck size={15} className="text-primary shrink-0" />
            {props.status?.target_configured
              ? "管理目标已保存在 Console 私有配置库，Admin Key 不会重新要求输入。"
              : "Admin Key 仅保存在后端本地配置库，页面不会读取或显示它。"}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
