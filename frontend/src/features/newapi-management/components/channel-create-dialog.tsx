import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type NewAPILocalGroup } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import { NewAPIChannelForm } from "./channel-form";

export function ChannelCreateDialog(props: {
  platformId: string;
  groups: NewAPILocalGroup[];
  baseURL: string;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const remoteGroups = useQuery({
    queryKey: ["newapi-remote-groups", props.platformId],
    queryFn: ({ signal }) => api.newAPIRemoteGroups(props.platformId, signal),
    retry: false,
  });
  const vault = useQuery({
    queryKey: ["auth-recovery-config"],
    queryFn: ({ signal }) => api.authRecoveryConfig(signal),
    retry: false,
  });
  const create = useMutation({
    mutationFn: (payload: Parameters<typeof api.createNewAPIChannel>[1]) =>
      api.createNewAPIChannel(props.platformId, payload),
    onSuccess: () => {
      toast.success("New API 渠道已创建");
      void client.invalidateQueries({ queryKey: ["newapi-channels", props.platformId] });
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "New API 渠道创建失败"),
  });
  const key = useMutation({
    mutationFn: (payload: Parameters<typeof api.createNewAPIChannelKey>[1]) =>
      api.createNewAPIChannelKey(props.platformId, payload),
    onError: (error) => notifyOperationError(error, "Sub2API 密钥创建失败"),
  });
  const models = useMutation({
    mutationFn: async (input: {
      payload: Parameters<typeof api.fetchNewAPIChannelModels>[1];
      signal?: AbortSignal;
    }) =>
      (await api.fetchNewAPIChannelModels(props.platformId, input.payload, input.signal)).models,
  });
  const writing = create.isPending || key.isPending;
  const ready = !!remoteGroups.data && !!vault.data;
  const loading = remoteGroups.isPending || vault.isPending;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !writing) props.onClose();
      }}
    >
      <DialogContent width="wide" height="tall">
        <DialogHeader>
          <DialogTitle>新增渠道</DialogTitle>
          <DialogDescription>
            选择 Sub2API 分组并创建密钥，再配置渠道模型与 New API 分组。
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          {remoteGroups.error && (
            <QueryErrorToast error={remoteGroups.error} fallback="New API 分组读取失败" />
          )}
          {vault.error && <QueryErrorToast error={vault.error} fallback="密码箱账号读取失败" />}
          {!ready && loading && <ContentLoading label="正在读取渠道创建配置" />}
          {!ready && !loading && (
            <ContentRetry
              pending={remoteGroups.isFetching || vault.isFetching}
              onRetry={() => {
                void remoteGroups.refetch();
                void vault.refetch();
              }}
            />
          )}
          {ready && (
            <NewAPIChannelForm
              groups={props.groups}
              newAPIGroups={remoteGroups.data.groups}
              sub2APIBaseURL={props.baseURL}
              vaultEntries={vault.data.vault_entries}
              pending={create.isPending}
              creatingKey={key.isPending}
              fetchingModels={models.isPending}
              onCreateKey={async (payload) => {
                try {
                  return await key.mutateAsync(payload);
                } finally {
                  key.reset();
                }
              }}
              onFetchModels={(payload, signal) => models.mutateAsync({ payload, signal })}
              onSubmit={async (payload) => {
                await create.mutateAsync(payload);
              }}
            />
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={writing} onClick={props.onClose}>
            取消
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
