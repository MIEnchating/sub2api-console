import { useEffect, useId, useRef, useState, type ReactElement } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { toast } from "sonner";
import {
  api,
  ApiError,
  type WorkbenchLoginProfile,
  type WorkbenchLoginProfileInput,
  type WorkbenchSourceProfile,
  type WorkbenchSourceProfileIdentity,
  type WorkbenchSourceProfileInput,
} from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";
import {
  oauthLoginDefaults,
  oauthLoginInput,
  oauthLoginSchema,
  type OAuthLoginValues,
} from "../lib/oauth-login-schema";
import { WorkbenchOAuthLoginFields } from "./workbench-oauth-login-fields";

export function WorkbenchLoginProfileEditor(props: {
  accountID?: string;
  profile?: WorkbenchLoginProfile | WorkbenchSourceProfile;
  source?: WorkbenchSourceProfileIdentity;
  onClose: () => void;
}): ReactElement {
  const client = useQueryClient();
  const formID = useId();
  const [confirm, setConfirm] = useState(false);
  const input = useRef<WorkbenchLoginProfileInput | WorkbenchSourceProfileInput | null>(null);
  const mounted = useRef(true);
  const local = !!props.source || (!!props.profile && "scope" in props.profile);
  const [expired, setExpired] = useState(false);
  const defaults = useRef<OAuthLoginValues>({
    ...oauthLoginDefaults,
    email: props.profile?.email ?? props.source?.email ?? "",
    workspace_id: props.profile?.workspace_id ?? props.source?.workspace_id ?? "",
  });
  const form = useForm<OAuthLoginValues>({
    resolver: zodResolver(oauthLoginSchema),
    defaultValues: defaults.current,
  });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      input.current = null;
      form.reset(defaults.current);
    };
  }, [form]);
  useEffect(() => {
    if (!props.source) return;
    const remaining = Date.parse(props.source.expires_at) - Date.now();
    const timer = setTimeout(
      () => setExpired(true),
      Number.isFinite(remaining) ? Math.max(0, remaining) : 0,
    );
    return () => clearTimeout(timer);
  }, [props.source]);
  const save = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const value = input.current;
      input.current = null;
      if (!value) throw new Error("登录资料已清空，请重新填写");
      if ("scope" in value) return api.saveWorkbenchSourceProfile(value);
      return api.saveWorkbenchLoginProfile(value);
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: workbenchKeys.profiles });
      void client.invalidateQueries({ queryKey: ["account-workbench", "source-profiles"] });
      toast.success("登录资料已保存");
      if (mounted.current) props.onClose();
    },
    onError: (error) => {
      notifyOperationError(error, "登录资料保存失败，请重新填写并确认账号身份");
      if (error instanceof ApiError && error.status === 409) {
        void client.invalidateQueries({ queryKey: workbenchKeys.profiles });
        void client.invalidateQueries({ queryKey: ["account-workbench", "source-profiles"] });
        if (mounted.current) props.onClose();
        return;
      }
      if (mounted.current) setConfirm(false);
    },
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{props.profile ? "替换登录资料" : "新增登录资料"}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <p className="mb-3 text-sm wrap-anywhere">
            {local
              ? `官方用户 ID：${props.profile?.user_id ?? props.source?.user_id}；工作区：${defaults.current.workspace_id}`
              : `账号 ID：${props.accountID}`}
            {props.profile ? `；资料版本：${props.profile.revision}` : ""}
          </p>
          <form id={formID} onSubmit={form.handleSubmit(() => setConfirm(true))}>
            <fieldset disabled={save.isPending || expired} className="grid min-w-0 gap-3">
              <WorkbenchOAuthLoginFields form={form} identityReadOnly={local || !!props.profile} />
            </fieldset>
          </form>
          {expired ? (
            <p role="status" className="text-sm">
              身份来源已过期，请重新选择来源
            </p>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            取消
          </Button>
          <Button type="submit" form={formID} disabled={save.isPending || expired}>
            <Save aria-hidden="true" />
            保存登录资料
          </Button>
        </DialogFooter>
      </DialogContent>
      <ConfirmActionDialog
        open={confirm}
        title="确认保存登录资料"
        description={`将所填密码、TOTP 密钥、邮箱、接码及登录代理配置保存到服务器私有资料，并绑定${local ? `官方用户 ${props.profile?.user_id ?? props.source?.user_id}（${defaults.current.email}）` : `账号 ID ${props.accountID}`}。${props.profile ? "本次完整替换原资料，未填写的密码、密钥和配置将被清除。" : ""}`}
        confirmLabel="确认保存到服务器"
        pending={save.isPending}
        confirmDisabled={expired}
        onOpenChange={setConfirm}
        onConfirm={() => {
          const login = oauthLoginInput(form.getValues()).login;
          if (!login || expired) return;
          if (local)
            input.current = {
              scope: "local-export",
              id: props.profile?.id,
              revision: props.profile?.revision,
              source: props.source?.source,
              source_revision: props.source?.source_revision,
              login,
              confirmed: true,
            };
          else if (props.accountID)
            input.current = {
              id: props.profile?.id,
              account_id: props.accountID,
              revision: props.profile?.revision ?? 0,
              confirmed: true,
              login,
            };
          form.reset(defaults.current);
          save.mutate();
        }}
      />
    </Dialog>
  );
}
