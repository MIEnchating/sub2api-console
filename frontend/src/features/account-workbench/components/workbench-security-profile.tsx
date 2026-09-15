import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type WorkbenchLoginProfile } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";

export function WorkbenchSecurityProfile(props: {
  accountId: string;
  securityId?: string;
  batchId?: string;
}): ReactElement {
  const client = useQueryClient();
  const [confirmation, setConfirmation] = useState<WorkbenchLoginProfile | null>(null);
  const profiles = useQuery({
    queryKey: ["account-workbench", "profiles"],
    queryFn: api.workbenchLoginProfiles,
  });
  const profile = profiles.data?.find((item) => item.account_id === props.accountId);
  const save = useMutation({
    mutationFn: (value: { id: string; revision: number }) =>
      api.applyWorkbenchSecurityToProfile({
        profile_id: value.id,
        revision: value.revision,
        security_id: props.securityId,
        batch_id: props.batchId,
        account_id: props.accountId,
        confirmed: true,
      }),
    onSuccess: () => {
      setConfirmation(null);
      toast.success("账号安全结果已更新至登录资料");
      void client.invalidateQueries({ queryKey: ["account-workbench", "profiles"] });
    },
    onError: (error) => notifyOperationError(error, "安全结果未写入登录资料，请刷新资料后重试"),
  });
  if (profiles.isPending) return <ContentLoading compact label="正在读取登录资料" />;
  if (!profiles.data)
    return <ContentRetry pending={profiles.isFetching} onRetry={() => void profiles.refetch()} />;
  if (!profile) return <p className="text-sm text-muted-foreground">此账号尚未保存登录资料。</p>;
  return (
    <>
      <Button
        variant="outline"
        disabled={save.isPending || save.isSuccess}
        onClick={() => setConfirmation(profile)}
      >
        {save.isSuccess ? "已更新登录资料" : "更新已保存的登录资料"}
      </Button>
      <ConfirmActionDialog
        open={confirmation !== null}
        title="确认更新登录资料"
        description={`账号 ID：${props.accountId}，邮箱：${confirmation?.email ?? ""}。将用本次成功的安全结果替换该资料中的对应密码或 TOTP 密钥。资料版本：${confirmation?.revision ?? ""}。`}
        confirmLabel="确认更新资料"
        pending={save.isPending}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={() => {
          if (confirmation) save.mutate({ id: confirmation.id, revision: confirmation.revision });
        }}
      />
    </>
  );
}
