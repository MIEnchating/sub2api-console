import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, type KumaMonitor, type KumaWriteInput } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import { PageActions } from "@/components/page-actions";
import { RefreshButton } from "@/components/refresh-button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { CirclePlus, ExternalLink, FolderPlus } from "lucide-react";
import { notifyOperationError } from "@/lib/operation-feedback";
import {
  actionLabels,
  kumaConfigKey,
  kumaMonitorsKey,
  kumaQueryKey,
  kumaTemplatesKey,
} from "../constants";
import type { MonitorValues } from "../lib/schemas";
import { useTaskCompletion } from "../hooks/use-task-completion";
import { MonitorDialog } from "./monitor-dialog";
import { MonitorWorkspace } from "./monitor-workspace";

type Confirmation = { monitor: KumaMonitor; action: keyof typeof actionLabels };
const emptyMonitors: KumaMonitor[] = [];
export function UptimeKumaPage() {
  const client = useQueryClient();
  const task = useTaskCompletion();
  const configQuery = useQuery({ queryKey: kumaConfigKey, queryFn: api.kumaConfig, retry: false });
  const config = configQuery.data;
  const monitorsQuery = useQuery({
    queryKey: [...kumaMonitorsKey, config?.revision],
    queryFn: api.kumaMonitors,
    enabled: !!config?.api_key_configured,
    retry: false,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  });
  const [editor, setEditor] = useState<{
    monitor: KumaMonitor | null;
    initialType?: "group";
  } | null>(null);
  const templatesQuery = useQuery({
    queryKey: kumaTemplatesKey,
    queryFn: api.kumaTemplates,
    enabled: !!editor && editor.initialType !== "group" && !!config?.management_configured,
    retry: false,
  });
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const refresh = async (): Promise<void> => {
    await client.invalidateQueries({ queryKey: kumaQueryKey });
  };
  const write = useMutation({
    mutationFn: (input: { id: number; value: KumaWriteInput }) =>
      api.writeKumaMonitor(input.id, input.value).then(task.wait),
    onSuccess: async () => {
      setEditor(null);
      setConfirmation(null);
      toast.success("监控操作已完成");
      await refresh();
    },
    onError: async (error) => {
      if (!editor) notifyOperationError(error, "监控操作失败");
      await refresh();
    },
  });
  const pending = write.isPending;
  const stale =
    !monitorsQuery.data ||
    monitorsQuery.isError ||
    monitorsQuery.isFetching ||
    monitorsQuery.data.config.revision !== config?.revision;
  const monitors = monitorsQuery.data?.monitors ?? emptyMonitors;
  const submitMonitor = (values: MonitorValues): void => {
    if (!config || !editor) return;
    write.mutate({
      id: editor.monitor?.id ?? 0,
      value: {
        config_revision: config.revision,
        revision: editor.monitor?.revision ?? "",
        action: editor.monitor ? "edit" : "create",
        monitor: values,
      },
    });
  };
  const confirmAction = (): void => {
    if (!config || !confirmation) return;
    write.mutate({
      id: confirmation.monitor.id,
      value: {
        config_revision: config.revision,
        revision: confirmation.monitor.revision,
        action: confirmation.action,
      },
    });
  };
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
                  setEditor({ monitor: null, initialType: "group" });
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
                  setEditor({ monitor: null });
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
      {templatesQuery.error && (
        <QueryErrorToast
          error={templatesQuery.error}
          fallback="模板读取失败，请关闭编辑窗口后刷新重试"
        />
      )}
      <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
        {configQuery.isPending && <PageLoadingSkeleton label="正在读取接入配置…" variant="table" />}
        {config && !config.api_key_configured && (
          <Card size="sm">
            <CardHeader>
              <CardTitle>接入 Uptime Kuma</CardTitle>
              <CardDescription>
                填写服务地址和 API 密钥即可查看监控状态；补充管理账号后可管理监控项。
              </CardDescription>
            </CardHeader>
            <CardContent>请在侧栏「Uptime Kuma → 接入配置」中完成接入。</CardContent>
          </Card>
        )}
        {config?.api_key_configured && (
          <>
            {monitorsQuery.isPending && (
              <PageLoadingSkeleton label="正在读取监控项…" variant="table" />
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
                  setEditor({ monitor });
                }}
                onAction={(monitor, action) => setConfirmation({ monitor, action })}
              />
            )}
          </>
        )}
      </div>
      {editor && (
        <MonitorDialog
          monitor={editor.monitor}
          initialType={editor.initialType}
          monitors={monitors}
          templates={templatesQuery.data}
          templatesPending={templatesQuery.isPending || templatesQuery.isError}
          pending={write.isPending}
          task={task.task}
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
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        onConfirm={confirmAction}
      />
    </PageLayout>
  );
}
