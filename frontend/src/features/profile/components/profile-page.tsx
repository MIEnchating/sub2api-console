import { useEffect, useRef, type ReactNode } from "react";
import { FieldError } from "@/components/field-error";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Save, ShieldCheck, UserRound } from "lucide-react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { api, type SessionStatus } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { notifyOperationError } from "@/lib/operation-feedback";

const profileSchema = z
  .object({
    username: z.string().trim().min(2, "账号至少 2 个字符").max(80, "账号不能超过 80 个字符"),
    current_password: z.string().min(1, "请输入当前密码"),
    new_password: z.string().max(256, "新密码不能超过 256 个字符"),
    confirm_password: z.string().max(256, "确认密码不能超过 256 个字符"),
  })
  .superRefine((value, context) => {
    if (!value.new_password && !value.confirm_password) return;
    if (value.new_password.length < 10) {
      context.addIssue({ code: "custom", path: ["new_password"], message: "新密码至少 10 个字符" });
    }
    if (value.new_password === value.current_password) {
      context.addIssue({
        code: "custom",
        path: ["new_password"],
        message: "新密码不能与当前密码相同",
      });
    }
    if (value.new_password !== value.confirm_password) {
      context.addIssue({
        code: "custom",
        path: ["confirm_password"],
        message: "两次输入的新密码不一致",
      });
    }
  });

type ProfileForm = z.infer<typeof profileSchema>;

function Field(props: { id: string; label: string; error?: string; children: ReactNode }) {
  return (
    <div className="grid gap-1.5">
      <label className="text-sm font-medium" htmlFor={props.id}>
        {props.label}
      </label>
      {props.children}
      <FieldError id={`${props.id}-error`} message={props.error} />
    </div>
  );
}

function accountInitials(username: string): string {
  return Array.from(username.trim()).slice(0, 2).join("").toLocaleUpperCase() || "AD";
}

function emptyProfileForm(username: string): ProfileForm {
  return { username, current_password: "", new_password: "", confirm_password: "" };
}

export function ProfilePage() {
  const pendingPayload = useRef<Parameters<typeof api.updateProfile>[0] | null>(null);
  const queryClient = useQueryClient();
  const session = useQuery({ queryKey: ["session"], queryFn: api.session });
  const username = session.data?.username ?? "";
  const form = useForm<ProfileForm>({
    resolver: zodResolver(profileSchema),
    defaultValues: emptyProfileForm(""),
  });

  useEffect(() => {
    if (username) form.reset(emptyProfileForm(username));
  }, [form, username]);

  function saveSession(saved: SessionStatus) {
    queryClient.setQueryData(["session"], saved);
    void queryClient.invalidateQueries({ queryKey: ["setup-status"] });
    void queryClient.invalidateQueries({ queryKey: ["config"] });
  }

  const updateProfile = useMutation({
    mutationFn: () => {
      const payload = pendingPayload.current;
      pendingPayload.current = null;
      if (!payload) throw new Error("提交内容已失效，请重新填写后保存");
      return api.updateProfile(payload);
    },
    onSuccess: (saved) => {
      saveSession(saved);
      form.reset(emptyProfileForm(saved.username ?? ""));
      toast.success("账号信息已保存，其他登录会话已退出");
    },
    onError: (error) => notifyOperationError(error, "账号信息保存失败"),
  });
  const unavailable = updateProfile.isPending || session.isError || !session.data?.authenticated;
  const submit = form.handleSubmit((values) => {
    if (unavailable || pendingPayload.current) return;
    pendingPayload.current = {
      username: values.username,
      current_password: values.current_password,
      ...(values.new_password ? { new_password: values.new_password } : {}),
    };
    updateProfile.mutate();
  });
  const changingPassword = Boolean(form.watch("new_password"));

  return (
    <PageLayout>
      <PageHeading
        eyebrow="ACCOUNT / PROFILE"
        title="个人信息"
        description="管理控制台登录账号和密码。"
      />
      {session.error ? <QueryErrorToast error={session.error} fallback="个人信息读取失败" /> : null}
      {session.isError ? (
        <ContentRetry onRetry={() => void session.refetch()} pending={session.isFetching} />
      ) : null}
      <div className="grid items-start gap-4 lg:grid-cols-[minmax(240px,0.72fr)_minmax(0,1.28fr)]">
        <Card className="lg:sticky lg:top-4">
          <CardHeader className="border-b">
            <CardTitle className="flex items-center gap-2">
              <UserRound className="text-primary" />
              管理员资料
            </CardTitle>
          </CardHeader>
          <CardContent>
            {session.isLoading ? (
              <div className="flex items-center gap-3">
                <Skeleton className="size-12 rounded-lg" />
                <div className="flex-1">
                  <Skeleton className="h-5 w-28" />
                  <Skeleton className="mt-2 h-4 w-20" />
                </div>
              </div>
            ) : (
              <div className="flex items-center gap-3">
                <div className="bg-primary/12 text-primary flex size-12 shrink-0 items-center justify-center rounded-lg text-sm font-semibold ring-1 ring-primary/20">
                  {accountInitials(username)}
                </div>
                <div className="min-w-0">
                  <p className="truncate text-base font-semibold">{username || "未读取"}</p>
                  <Badge className="mt-1.5" variant="secondary">
                    管理员
                  </Badge>
                </div>
              </div>
            )}
            <div className="text-muted-foreground mt-5 flex items-start gap-2 border-t pt-4 text-xs leading-5">
              <ShieldCheck className="text-primary mt-0.5 size-4 shrink-0" />
              账号凭据保存在本地私有配置库，不会写入业务数据库。
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b">
            <CardTitle className="flex items-center gap-2">
              <KeyRound className="text-primary" />
              账号与密码
            </CardTitle>
          </CardHeader>
          <CardContent>
            <form className="grid gap-4" onSubmit={submit}>
              <Field
                id="profile-username"
                label="账号"
                error={form.formState.errors.username?.message}
              >
                <Input
                  id="profile-username"
                  disabled={unavailable}
                  autoComplete="username"
                  aria-invalid={Boolean(form.formState.errors.username)}
                  aria-describedby={
                    form.formState.errors.username ? "profile-username-error" : undefined
                  }
                  {...form.register("username")}
                />
              </Field>
              <Field
                id="profile-current-password"
                label="当前密码"
                error={form.formState.errors.current_password?.message}
              >
                <Input
                  id="profile-current-password"
                  disabled={unavailable}
                  type="password"
                  autoComplete="current-password"
                  aria-invalid={Boolean(form.formState.errors.current_password)}
                  aria-describedby={
                    form.formState.errors.current_password
                      ? "profile-current-password-error"
                      : undefined
                  }
                  {...form.register("current_password")}
                />
              </Field>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field
                  id="profile-new-password"
                  label="新密码（可选）"
                  error={form.formState.errors.new_password?.message}
                >
                  <Input
                    id="profile-new-password"
                    disabled={unavailable}
                    type="password"
                    autoComplete="new-password"
                    placeholder="不修改请留空"
                    aria-invalid={Boolean(form.formState.errors.new_password)}
                    aria-describedby={
                      form.formState.errors.new_password ? "profile-new-password-error" : undefined
                    }
                    {...form.register("new_password")}
                  />
                </Field>
                <Field
                  id="profile-confirm-password"
                  label="确认新密码"
                  error={form.formState.errors.confirm_password?.message}
                >
                  <Input
                    id="profile-confirm-password"
                    type="password"
                    autoComplete="new-password"
                    disabled={unavailable || !changingPassword}
                    aria-invalid={Boolean(form.formState.errors.confirm_password)}
                    aria-describedby={
                      form.formState.errors.confirm_password
                        ? "profile-confirm-password-error"
                        : undefined
                    }
                    {...form.register("confirm_password")}
                  />
                </Field>
              </div>
              <Button className="justify-self-start" type="submit" disabled={unavailable}>
                <Save /> {updateProfile.isPending ? "保存中" : "保存修改"}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </PageLayout>
  );
}
