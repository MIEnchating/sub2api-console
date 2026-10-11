import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Unlink } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { api } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { notifyOperationError } from "@/lib/operation-feedback";

type Props = {
  host: string;
  bindingId: number;
  upstreamId: string;
  accountId: string;
  upstreamKeyId: string;
  groupId: string;
  groupName: string;
  disabled: boolean;
  onChanged?: () => void;
};

const refreshKeys = [
  "upstreams",
  "upstream-configuration",
  "upstream-groups",
  "upstream-delete-preview",
  "upstream-allocation",
  "onboarding-candidates",
  "onboarding-preparation",
  "onboarding-unbound-keys",
  "accounts",
  "logs",
];

export function StaleBindingCleanup(props: Props) {
  const identity = JSON.stringify([
    props.host,
    props.bindingId,
    props.upstreamId,
    props.accountId,
    props.upstreamKeyId,
    props.groupId,
  ]);
  return <BindingCleanupAction key={identity} {...props} />;
}

function BindingCleanupAction(props: Props) {
  const [open, setOpen] = useState(false);
  const mounted = useRef(false);
  const submitting = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const queryClient = useQueryClient();
  const identityReady = Boolean(
    props.upstreamId &&
    props.groupId &&
    props.upstreamKeyId &&
    props.accountId &&
    props.bindingId > 0,
  );
  const cleanup = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: (target: Props) =>
      api.cleanupUpstreamBinding(target.host, target.bindingId, {
        upstream_id: target.upstreamId,
        account_id: target.accountId,
        upstream_key_id: target.upstreamKeyId,
        upstream_group_id: target.groupId,
      }),
    onSuccess: () => {
      void Promise.all(
        refreshKeys.map((key) => queryClient.invalidateQueries({ queryKey: [key] })),
      );
      if (!mounted.current) return;
      setOpen(false);
      toast.success("失效绑定已清理");
      props.onChanged?.();
    },
    onError: (error) => {
      if (mounted.current) notifyOperationError(error, "清理失效绑定失败");
    },
    onSettled: () => {
      submitting.current = false;
    },
  });

  return (
    <>
      <TableActionButton
        label={identityReady ? "清理失效绑定" : "清理失效绑定：请先同步上游确认绑定身份"}
        tone="danger"
        disabled={props.disabled || cleanup.isPending || !identityReady}
        onClick={() => setOpen(true)}
      >
        <Unlink />
      </TableActionButton>
      <ConfirmActionDialog
        open={open}
        title="清理失效绑定"
        description={`确认清理账号 ${props.accountId} 与上游分组「${props.groupName}」的失效绑定（绑定 ID ${props.bindingId}）？仅移除这条关联记录，不删除账号、上游 Key 或其他绑定。`}
        confirmLabel="确认清理"
        pendingLabel="正在清理…"
        pending={cleanup.isPending}
        confirmDisabled={props.disabled || !identityReady}
        onOpenChange={setOpen}
        onConfirm={() => {
          if (submitting.current || props.disabled || !identityReady) return;
          submitting.current = true;
          cleanup.mutate({ ...props });
        }}
      />
    </>
  );
}
