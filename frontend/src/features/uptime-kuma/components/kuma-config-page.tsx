import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import { PageActions } from "@/components/page-actions";
import { RefreshButton } from "@/components/refresh-button";
import { TaskProgressState } from "@/components/task-startup-state";
import { Save, Unplug } from "lucide-react";
import { notifyOperationError } from "@/lib/operation-feedback";
import { kumaConfigKey, kumaMonitorsKey, kumaQueryKey } from "../constants";
import type { ConfigValues } from "../lib/schemas";
import { useTaskCompletion } from "../hooks/use-task-completion";
import { ConfigForm } from "./config-form";

export function KumaConfigPage() {
  const client = useQueryClient();
  const task = useTaskCompletion();
  const query = useQuery({
    queryKey: kumaConfigKey,
    queryFn: api.kumaConfig,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const config = query.data;
  const [disconnectOpen, setDisconnectOpen] = useState(false);
  const save = useMutation({
    mutationFn: (values: ConfigValues) =>
      api.saveKumaConfig({ ...values, revision: config!.revision }).then(task.wait),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: kumaQueryKey });
      toast.success("Uptime Kuma 接入配置已保存");
      save.reset();
    },
  });
  const disconnect = useMutation({
    mutationFn: () => api.disconnectKuma(config!.revision),
    onSuccess: async () => {
      setDisconnectOpen(false);
      save.reset();
      client.removeQueries({ queryKey: kumaMonitorsKey });
      await client.invalidateQueries({ queryKey: kumaConfigKey });
      toast.success("已断开接入，远端监控项保留");
    },
    onError: (error) => notifyOperationError(error, "断开接入失败"),
  });
  const pending = save.isPending || disconnect.isPending;
  return (
    <PageLayout>
      <PageHeading
        eyebrow="Uptime Kuma"
        title="接入配置"
        description="配置 Uptime Kuma 的服务地址、API 密钥和管理账号"
        action={
          <PageActions>
            <RefreshButton
              ariaLabel="刷新配置"
              pending={query.isFetching}
              disabled={pending}
              onClick={() => void query.refetch()}
            />
            {config?.api_key_configured && (
              <Button
                variant="destructive"
                disabled={pending || query.isError}
                onClick={() => setDisconnectOpen(true)}
              >
                <Unplug aria-hidden="true" />
                断开接入
              </Button>
            )}
            <Button type="submit" form="kuma-config" disabled={!config || pending || query.isError}>
              <Save aria-hidden="true" />
              {save.isPending ? "正在验证…" : "验证并保存"}
            </Button>
          </PageActions>
        }
      />
      <div className="grid min-w-0 gap-3">
        {query.error && <QueryErrorToast error={query.error} fallback="接入配置读取失败" />}
        {query.isPending && <PageLoadingSkeleton label="正在读取接入配置…" variant="form" />}
        {save.isPending && task.task && (
          <TaskProgressState message={task.task.message} progress={task.task.progress} />
        )}
        {config && (
          <>
            <ConfigForm
              key={config.revision}
              config={config}
              pending={pending || query.isError}
              error={save.error}
              onSubmit={(values) => save.mutate(values)}
            />
          </>
        )}
      </div>
      <ConfirmActionDialog
        open={disconnectOpen}
        title="断开 Uptime Kuma 接入"
        description="清除控制台保存的地址和凭据。远端监控项及历史记录会保留。"
        confirmLabel="断开接入"
        pending={disconnect.isPending}
        onOpenChange={setDisconnectOpen}
        onConfirm={() => disconnect.mutate()}
      />
    </PageLayout>
  );
}
