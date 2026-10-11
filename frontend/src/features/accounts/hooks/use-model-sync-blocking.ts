import { useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type AccountModelSyncPreview, type AccountModelSyncSettings } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";

type Scope = { accountIds: string[]; key: string };

export function useModelSyncBlocking(options: {
  accountIds: string[];
  onPreviewUpdated: (preview: AccountModelSyncPreview) => void;
}) {
  const queryClient = useQueryClient();
  const scopeKey = JSON.stringify(options.accountIds);
  const currentScope = useRef(scopeKey);
  currentScope.current = scopeKey;
  const mounted = useRef(false);
  const submitting = useRef(false);
  const refreshing = useRef(false);
  const controller = useRef<AbortController | null>(null);
  // Retain confirmed rules if loading the new catalog fingerprint fails.
  const [savedSettings, setSavedSettings] = useState<AccountModelSyncSettings | null>(null);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
    };
  }, [scopeKey]);
  const isCurrent = (scope: Scope): boolean =>
    mounted.current && currentScope.current === scope.key;
  const refresh = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: async (scope: Scope) => {
      if (!isCurrent(scope)) return null;
      const request = new AbortController();
      controller.current = request;
      const preview = await api.previewAccountModels(scope.accountIds, request.signal);
      if (request.signal.aborted || !isCurrent(scope)) return null;
      return preview;
    },
    onSuccess: (preview, scope) => {
      if (!preview || !isCurrent(scope)) return;
      options.onPreviewUpdated(preview);
      queryClient.setQueryData(["account-model-sync-preview", scope.accountIds], preview);
      setSavedSettings(null);
    },
    onError: (error, scope) => {
      if (!isCurrent(scope) || controller.current?.signal.aborted) return;
      notifyOperationError(error, "模型预览读取失败", {
        context: "全局屏蔽已保存，请重新读取模型预览后继续同步",
      });
    },
    onSettled: () => {
      refreshing.current = false;
    },
  });
  const block = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: async (input: Scope & { model: string }) => {
      await queryClient.cancelQueries({
        queryKey: ["account-model-sync-preview", input.accountIds],
        exact: true,
      });
      if (!isCurrent(input)) return null;
      return api.addAccountModelBlock(input.model);
    },
    onSuccess: async (settings, scope) => {
      if (!settings) return;
      // A concurrent writer may already have saved a newer list. Re-read rather
      // than replacing the shared settings cache with this response snapshot.
      await queryClient.cancelQueries({ queryKey: ["account-model-sync-settings"], exact: true });
      void queryClient.invalidateQueries({
        queryKey: ["account-model-sync-settings"],
        exact: true,
      });
      await queryClient.cancelQueries({ queryKey: ["account-model-sync-preview"] });
      await queryClient.invalidateQueries({
        queryKey: ["account-model-sync-preview"],
        refetchType: "none",
      });
      if (!isCurrent(scope)) return;
      setSavedSettings(settings);
      toast.success("已加入全局屏蔽模型，后续同步不再显示");
      refreshing.current = true;
      await refresh.mutateAsync(scope).catch(() => undefined);
    },
    onError: (error, scope) => {
      if (!isCurrent(scope)) return;
      notifyOperationError(error, "全局屏蔽模型保存失败", { context: "排除模型失败，请重试" });
    },
    onSettled: () => {
      submitting.current = false;
    },
  });
  return {
    block: (model: string): void => {
      if (!mounted.current || submitting.current || refreshing.current) return;
      submitting.current = true;
      block.mutate({ model, accountIds: [...options.accountIds], key: scopeKey });
    },
    refresh: (): void => {
      if (!mounted.current || submitting.current || refreshing.current) return;
      refreshing.current = true;
      refresh.mutate({ accountIds: [...options.accountIds], key: scopeKey });
    },
    pending: block.isPending || refresh.isPending,
    refreshFailed: refresh.isError,
    savedSettings,
  };
}
