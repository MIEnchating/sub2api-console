import { useEffect, useId, useState, type ReactElement } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { LogIn } from "lucide-react";
import type { WorkbenchOAuthStartInput } from "@/api";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  oauthLoginDefaults,
  oauthLoginInput,
  oauthLoginSchema,
  type OAuthLoginValues,
} from "../lib/oauth-login-schema";
import { WorkbenchOAuthLoginFields } from "./workbench-oauth-login-fields";
import { oauthProxyFormSchema, type OAuthProxyValues } from "../lib/oauth-proxy-schema";

export function WorkbenchOAuthStart(props: {
  disabled: boolean;
  retry: boolean;
  onStart: (input: WorkbenchOAuthStartInput) => void;
}): ReactElement {
  const [automatic, setAutomatic] = useState(false);
  const [recovery, setRecovery] = useState(false);
  const [open, setOpen] = useState(false);
  const formID = useId();
  const proxyForm = useForm<OAuthProxyValues>({
    resolver: zodResolver(oauthProxyFormSchema),
    defaultValues: { proxy_url: "" },
  });
  const form = useForm<OAuthLoginValues>({
    resolver: zodResolver(oauthLoginSchema),
    defaultValues: oauthLoginDefaults,
  });
  const close = (): void => {
    form.reset(oauthLoginDefaults);
    proxyForm.reset({ proxy_url: "" });
    setOpen(false);
  };
  useEffect(() => () => form.reset(oauthLoginDefaults), [form]);
  useEffect(() => () => proxyForm.reset({ proxy_url: "" }), [proxyForm]);
  return (
    <section aria-label="授权启动设置" className="@container min-w-0 rounded-lg border bg-card">
      <div
        className={cn("grid min-w-0 gap-3 p-3 sm:p-4", !automatic && "@2xl:grid-cols-2 @2xl:gap-6")}
      >
        {!automatic && (
          <FormField
            label="登录代理"
            htmlFor="oauth-manual-proxy"
            description="可选，留空时直接连接官方授权页面。"
            error={proxyForm.formState.errors.proxy_url?.message}
            reserveErrorSpace={false}
          >
            <Input
              id="oauth-manual-proxy"
              type="password"
              autoComplete="off"
              placeholder="可选，留空直接连接"
              disabled={props.disabled}
              aria-invalid={!!proxyForm.formState.errors.proxy_url}
              {...proxyForm.register("proxy_url")}
            />
          </FormField>
        )}
        <div className="grid min-w-0 content-start gap-3 @2xl:pt-1">
          <label className="flex items-start gap-2 text-sm">
            <Checkbox
              checked={automatic}
              disabled={props.disabled}
              onCheckedChange={(checked) => {
                setAutomatic(checked);
                proxyForm.reset({ proxy_url: "" });
                if (!checked) close();
              }}
            />
            <span className="min-w-0 wrap-anywhere">自动填写登录信息</span>
          </label>
          <label className="flex items-start gap-2 text-sm">
            <Checkbox checked={recovery} disabled={props.disabled} onCheckedChange={setRecovery} />
            <span className="min-w-0 wrap-anywhere">
              自动保存私有登录检查点（按原授权到期时间清除）
            </span>
          </label>
        </div>
      </div>
      <div className="flex flex-wrap justify-end gap-2 border-t px-3 py-3 sm:px-4">
        <Button
          className="w-full sm:w-auto"
          type="button"
          disabled={props.disabled}
          onClick={() => {
            if (automatic) setOpen(true);
            else
              void proxyForm.handleSubmit((values) => {
                proxyForm.reset({ proxy_url: "" });
                props.onStart({
                  ...(values.proxy_url ? { proxy_url: values.proxy_url } : {}),
                  ...(recovery ? { recovery_enabled: true } : {}),
                });
              })();
          }}
        >
          <LogIn aria-hidden="true" />
          {props.retry ? "重新授权登录" : "开始授权登录"}
        </Button>
      </div>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!value) close();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>自动填写登录信息</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <form
              id={formID}
              className="grid min-w-0 gap-3"
              onSubmit={form.handleSubmit((values) => {
                const input = oauthLoginInput(values);
                form.reset(oauthLoginDefaults);
                setOpen(false);
                props.onStart({ ...input, ...(recovery ? { recovery_enabled: true } : {}) });
              })}
            >
              <WorkbenchOAuthLoginFields form={form} />
            </form>
          </DialogBody>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={close}>
              取消
            </Button>
            <Button type="submit" form={formID}>
              <LogIn aria-hidden="true" />
              开始授权登录
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
