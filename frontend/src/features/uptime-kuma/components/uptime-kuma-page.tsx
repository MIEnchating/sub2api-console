import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type KumaMonitor, type KumaWriteInput } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { PageActions } from "@/components/page-actions";
import { RefreshButton } from "@/components/refresh-button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { CirclePlus, ExternalLink, FolderPlus } from "lucide-react";
import { notifyOperationError } from "@/lib/operation-feedback";
import { actionLabels, kumaConfigKey, kumaMonitorsKey, kumaQueryKey } from "../constants";
import type { MonitorValues } from "../lib/schemas";
import { useTaskCompletion } from "../hooks/use-task-completion";
import { MonitorEditor } from "./monitor-editor";
import { MonitorWorkspace } from "./monitor-workspace";
import { KumaTaskProgress } from "./kuma-task-progress";
import { isKumaTaskCancellation } from "../lib/task-cancellation";

type Confirmation = {
  monitor: KumaMonitor;
  action: keyof typeof actionLabels;
  configRevision: number;
};
const emptyMonitors: KumaMonitor[] = [];
export function UptimeKumaPage() {
  const client = useQueryClient();
  const task = useTaskCompletion();
  const configQuery = useQuery({
    queryKey: kumaConfigKey,
    queryFn: ({ signal }) => api.kumaConfig(signal),
    retry: false,
  });
  const config = configQuery.data;
  const monitorsQuery = useQuery({
    queryKey: [...kumaMonitorsKey, config?.revision],
    queryFn: ({ signal }) => api.kumaMonitors(signal),
    enabled: !!config?.api_key_configured,
    retry: false,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  });
  const [editor, setEditor] = useState<{
    monitor: KumaMonitor | null;
    initialType?: "group";
    configRevision: number;
  } | null>(null);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const refresh = async (): Promise<void> => {
    await client.invalidateQueries({ queryKey: kumaQueryKey });
  };
  const write = useMutation({
    mutationFn: (input: { id: number; value: KumaWriteInput }) => {
      task.prepare();
      return api.writeKumaMonitor(input.id, input.value).then(task.wait);
    },
    onSuccess: async () => {
      setEditor(null);
      setConfirmation(null);
      toast.success("监控操作已完成");
      await refresh();
    },
    onError: async (error) => {
      if (isKumaTaskCancellation(error)) {
        setEditor(null);
        setConfirmation(null);
        toast.info(error.message);
      } else if (!editor) notifyOperationError(error, "监控操作失败");
      await refresh();
    },
  });
  const pending = write.isPending;
  const stale =
    configQuery.isError ||
    !monitorsQuery.data ||
    monitorsQuery.isError ||
    monitorsQuery.isFetching ||
    monitorsQuery.data.config.revision !== config?.revision;
  const monitors = monitorsQuery.data?.monitors ?? emptyMonitors;
  const submitMonitor = (values: MonitorValues): void => {
    if (!config || !editor) return;
    if (!canSubmit(editor.configRevision)) return;
    write.mutate({
      id: editor.monitor?.id ?? 0,
      value: {
        config_revision: editor.configRevision,
        revision: editor.monitor?.revision ?? "",
        action: editor.monitor ? "edit" : "create",
        monitor: values,
      },
    });
  };
  const confirmAction = (): void => {
    if (!config || !confirmation) return;
    if (!canSubmit(confirmation.configRevision)) return;
    write.mutate({
      id: confirmation.monitor.id,
      value: {
        config_revision: confirmation.configRevision,
        revision: confirmation.monitor.revision,
        action: confirmation.action,
      },
    });
  };
  function canSubmit(configRevision: number): boolean {
    if (configRevision !== config?.revision) {
      notifyOperationError("监控平台配置已更改，请关闭窗口并重新打开后操作", "监控操作未提交");
      return false;
    }
    if (stale || configQuery.isFetching) {
      notifyOperationError("监控数据尚未读取完成，请刷新后重试", "监控操作未提交");
      return false;
    }
    return true;
  }
  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="Uptime Kuma"
        title="监控管理"
        description="接入并管理监控项"
        action={
          <PageActions>
            {config?.api_key_configured && (
              <Button
                role="link"
                variant="outline"
                render={
                  <a href={`${config.base_url}/dashboard`} target="_blank" rel="noreferrer" />
                }
              >
                <ExternalLink aria-hidden="true" />
                原仪表盘
              </Button>
            )}
            <RefreshButton
              ariaLabel="刷新监控项"
              pending={configQuery.isFetching || monitorsQuery.isFetching}
              disabled={pending}
              onClick={() => void refresh()}
            />
            {config?.management_configured && (
              <Button
                variant="outline"
                disabled={pending || stale}
                onClick={() => {
                  write.reset();
                  setEditor({
                    monitor: null,
                    initialType: "group",
                    configRevision: config.revision,
                  });
                }}
              >
                <FolderPlus aria-hidden="true" />
                新增分组
              </Button>
            )}
            {config?.management_configured && (
              <Button
                disabled={pending || stale}
                onClick={() => {
                  write.reset();
                  setEditor({ monitor: null, configRevision: config.revision });
                }}
              >
                <CirclePlus aria-hidden="true" />
                新增监控项
              </Button>
            )}
          </PageActions>
        }
      />
      {(configQuery.error || monitorsQuery.error) && (
        <QueryErrorToast
          error={configQuery.error ?? monitorsQuery.error}
          fallback="Uptime Kuma 数据读取失败"
        />
      )}
      <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
        {configQuery.isPending && (
          <PageLoadingSkeleton label="正在读取接入配置…" variant="table" fill />
        )}
        {!config && configQuery.isError && (
          <ContentRetry
            pending={configQuery.isFetching}
            onRetry={() => void configQuery.refetch()}
          />
        )}
        {config && !config.api_key_configured && (
          <Card size="sm">
            <CardHeader>
              <CardTitle>接入 Uptime Kuma</CardTitle>
              <CardDescription>
                填写服务地址和 API 密钥即可查看监控状态；补充管理账号后可管理监控项。
              </CardDescription>
            </CardHeader>
            <CardContent>请在系统设置「监控平台」中完成接入。</CardContent>
          </Card>
        )}
        {config?.api_key_configured && (
          <>
            {monitorsQuery.isPending && (
              <PageLoadingSkeleton label="正在读取监控项…" variant="table" fill />
            )}
            {!monitorsQuery.data && monitorsQuery.isError && (
              <ContentRetry
                pending={monitorsQuery.isFetching}
                onRetry={() => void monitorsQuery.refetch()}
              />
            )}
            {monitorsQuery.data?.warning && (
              <Card size="sm">
                <CardContent role="status">{monitorsQuery.data.warning}</CardContent>
              </Card>
            )}
            {monitorsQuery.data && (
              <MonitorWorkspace
                monitors={monitors}
                management={config.management_configured}
                disabled={pending || stale}
                onEdit={(monitor) => {
                  write.reset();
                  setEditor({ monitor, configRevision: config.revision });
                }}
                onAction={(monitor, action) =>
                  setConfirmation({ monitor, action, configRevision: config.revision })
                }
              />
            )}
          </>
        )}
      </div>
      {editor && (
        <MonitorEditor
          management={!!config?.management_configured}
          monitor={editor.monitor}
          initialType={editor.initialType}
          monitors={monitors}
          pending={write.isPending}
          task={task.task}
          starting={task.starting}
          error={write.error}
          onClose={() => setEditor(null)}
          onSubmit={submitMonitor}
        />
      )}
      <ConfirmActionDialog
        open={!!confirmation}
        title={`${confirmation ? actionLabels[confirmation.action] : "管理"}监控项`}
        description={
          confirmation
            ? `即将${actionLabels[confirmation.action]}「${confirmation.monitor.name}」（ID ${confirmation.monitor.id}）。${confirmation.action === "delete" ? "删除会移除该项及其历史记录，无法撤销。非空分组需要先移出子项。" : "分组的暂停或恢复会影响子项的有效监控状态。"}`
            : ""
        }
        confirmLabel={confirmation ? actionLabels[confirmation.action] : "确认"}
        pending={write.isPending}
        pendingContent={<KumaTaskProgress task={task.task} starting={task.starting} />}
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={confirmAction}
      />
    </PageLayout>
  );
}
