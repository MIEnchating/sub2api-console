import { useState, type ReactElement } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api, type WorkbenchSourceProfile } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";

export function WorkbenchSourceSecurityProfile(props: {
  securityId: string;
  userId: string;
  workspaceId: string;
}): ReactElement {
  const [confirmation, setConfirmation] = useState<WorkbenchSourceProfile | null>(null);
  const profiles = useQuery({
    queryKey: ["account-workbench", "source-profiles"],
    queryFn: (context) => api.workbenchSourceProfiles(context.signal),
    gcTime: 0,
  });
  const profile = profiles.data?.find(
    (item) => item.user_id === props.userId && item.workspace_id === props.workspaceId,
  );
  const save = useMutation({
    mutationFn: async (selected: WorkbenchSourceProfile) => {
      await api.applyWorkbenchSecurityToSourceProfile(selected.id, {
        revision: selected.revision,
        security_id: props.securityId,
        confirmed: true,
      });
    },
    onSuccess: () => {
      setConfirmation(null);
      void profiles.refetch();
    },
    onError: (error) => {
      setConfirmation(null);
      notifyOperationError(error, "本地登录资料未更新，请重新核对版本和官方身份");
      void profiles.refetch();
    },
  });
  if (profiles.isPending) return <ContentLoading label="正在核对本地登录资料" compact />;
  if (!profiles.data)
    return <ContentRetry pending={profiles.isFetching} onRetry={() => void profiles.refetch()} />;
  if (!profile)
    return <p className="text-sm text-muted-foreground">此官方账号尚未保存本地登录资料</p>;
  return (
    <>
      <Button
        variant="outline"
        disabled={profiles.isError || profiles.isFetching || save.isPending || save.isSuccess}
        onClick={() => setConfirmation(profile)}
      >
        {save.isSuccess ? "已更新本地登录资料" : "更新本地登录资料"}
      </Button>
      <ConfirmActionDialog
        open={confirmation !== null}
        title="更新本地登录资料"
        description={`将本次成功设置的密码或 TOTP 写入 ${confirmation?.email ?? ""} 的资料（官方用户 ${confirmation?.user_id ?? ""}，工作区 ${confirmation?.workspace_id ?? ""}，版本 ${confirmation?.revision ?? ""}）。`}
        confirmLabel="确认写入本地资料"
        pending={save.isPending}
        confirmDisabled={profiles.isError || profiles.isFetching}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={() => {
          if (confirmation) save.mutate(confirmation);
        }}
      />
    </>
  );
}
