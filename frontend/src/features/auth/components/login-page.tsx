import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import {
  ArrowRight,
  CircleAlert,
  Eye,
  EyeOff,
  LoaderCircle,
  LockKeyhole,
  UserRound,
} from "lucide-react";
import { useId, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";
import { api } from "@/api";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { notifyOperationError } from "@/lib/operation-feedback";
import { loginSchema, type LoginForm } from "../lib/login-schema";
import { LoginShell } from "./login-shell";

export type LoginPageProps = {
  onLogin: () => void;
  reason?: string | null;
  theme?: "light" | "dark";
  onThemeChange?: () => void;
};

export function LoginPage(props: LoginPageProps): ReactElement {
  const fieldID = useId();
  const [passwordVisible, setPasswordVisible] = useState(false);
  const form = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: "", password: "" },
  });
  const login = useMutation({
    mutationFn: api.login,
    networkMode: "always",
    gcTime: 0,
    retry: false,
    onSuccess: () => {
      setPasswordVisible(false);
      form.reset();
      props.onLogin();
    },
    onError: (error: unknown) => notifyOperationError(error, "登录失败"),
    onSettled: (): void => login.reset(),
  });
  const submit = form.handleSubmit((values) => {
    if (!login.isPending) login.mutate(values);
  });
  const passwordLabel = passwordVisible ? "隐藏密码" : "显示密码";

  return (
    <LoginShell theme={props.theme} onThemeChange={props.onThemeChange}>
      <section className="w-full min-w-0" aria-labelledby={`${fieldID}-title`}>
        <div className="mb-6 lg:mb-8">
          <h1 id={`${fieldID}-title`} className="text-lg font-semibold lg:text-2xl">
            登录
          </h1>
        </div>
        {props.reason ? (
          <div
            className="mb-5 flex min-w-0 items-start gap-2 rounded-lg border border-warning/35 bg-warning/10 px-3 py-2.5 text-sm text-warning"
            role="alert"
          >
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <span>{props.reason}</span>
          </div>
        ) : null}
        <form
          className="grid gap-2 [&_[data-slot=field-error]]:h-5 [&_[data-slot=field-error]]:min-h-5"
          onSubmit={submit}
          aria-label="登录"
          aria-busy={login.isPending}
        >
          <FormField
            reserveErrorSpace
            label="账号"
            htmlFor={`${fieldID}-username`}
            error={form.formState.errors.username?.message}
          >
            <div className="relative">
              <UserRound
                className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                id={`${fieldID}-username`}
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                disabled={login.isPending}
                aria-invalid={Boolean(form.formState.errors.username)}
                {...form.register("username")}
                className="pr-3 pl-9"
              />
            </div>
          </FormField>
          <FormField
            reserveErrorSpace
            label="密码"
            htmlFor={`${fieldID}-password`}
            error={form.formState.errors.password?.message}
          >
            <div className="relative">
              <LockKeyhole
                className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                id={`${fieldID}-password`}
                type={passwordVisible ? "text" : "password"}
                autoComplete="current-password"
                disabled={login.isPending}
                aria-invalid={Boolean(form.formState.errors.password)}
                {...form.register("password")}
                className="pr-10 pl-9"
              />
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      className="absolute top-0 right-0 text-muted-foreground"
                      aria-label={passwordLabel}
                      aria-pressed={passwordVisible}
                      aria-controls={`${fieldID}-password`}
                      disabled={login.isPending}
                      onClick={() => setPasswordVisible(!passwordVisible)}
                    />
                  }
                >
                  {passwordVisible ? <EyeOff aria-hidden="true" /> : <Eye aria-hidden="true" />}
                </TooltipTrigger>
                <TooltipContent>{passwordLabel}</TooltipContent>
              </Tooltip>
            </div>
          </FormField>
          <Button
            type="submit"
            className="mt-1 w-full"
            disabled={login.isPending}
            aria-busy={login.isPending}
          >
            {login.isPending ? (
              <LoaderCircle className="animate-spin" aria-hidden="true" />
            ) : (
              <ArrowRight aria-hidden="true" />
            )}
            {login.isPending ? "登录中…" : "登录"}
          </Button>
        </form>
      </section>
    </LoginShell>
  );
}
