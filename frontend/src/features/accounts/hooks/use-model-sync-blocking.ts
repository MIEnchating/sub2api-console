import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type AccountModelSyncPreview, type AccountModelSyncSettings } from "@/api";
import { notifyOperationError } from "@/lib/operation-feedback";
import { modelMatchesBlockPatterns } from "../lib/model-block-patterns";

export function useModelSyncBlocking(options: {
  accountIds: string[];
  onPreviewUpdated: (preview: AccountModelSyncPreview) => void;
}) {
  const queryClient = useQueryClient();
  // Retain confirmed rules even if loading the new catalog fingerprint fails.
  const [savedSettings, setSavedSettings] = useState<AccountModelSyncSettings | null>(null);
  const refresh = useMutation({
    mutationFn: () => api.previewAccountModels(options.accountIds),
    onSuccess: (preview) => {
      options.onPreviewUpdated(preview);
      queryClient.setQueryData(["account-model-sync-preview", options.accountIds], preview);
      setSavedSettings(null);
    },
    onError: (error) =>
      notifyOperationError(error, "模型预览读取失败", {
        context: "全局屏蔽已保存，请重新读取模型预览后继续同步",
      }),
  });
  const block = useMutation({
    onMutate: () =>
      queryClient.cancelQueries({ queryKey: ["account-model-sync-preview", options.accountIds] }),
    mutationFn: async (model: string) => {
      // Read the latest settings rather than overwriting rules using a cached preview.
      const settings = await api.accountModelSyncSettings();
      if (modelMatchesBlockPatterns(model, settings.blocked_patterns)) return settings;
      return api.updateAccountModelSyncSettings({
        blocked_patterns: [...settings.blocked_patterns, model],
      });
    },
    onSuccess: async (settings) => {
      setSavedSettings(settings);
      queryClient.setQueryData(["account-model-sync-settings"], settings);
      await queryClient.invalidateQueries({
        queryKey: ["account-model-sync-preview"],
        refetchType: "none",
      });
      toast.success("已加入全局屏蔽模型，后续同步不再显示");
      await refresh.mutateAsync().catch(() => undefined);
    },
    onError: (error) =>
      notifyOperationError(error, "全局屏蔽模型保存失败", {
        context: "排除模型失败，请重试",
      }),
  });
  return {
    block: block.mutate,
    refresh: refresh.mutate,
    pending: block.isPending || refresh.isPending,
    refreshFailed: refresh.isError,
    savedSettings,
  };
}
