import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Save, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type Task, type WorkbenchExportMetadata, type WorkbenchScope } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";
import { WorkbenchRegeneration } from "./workbench-regeneration";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchArtifactProfile } from "./workbench-artifact-profile";

export function WorkbenchExportArtifacts(props: { scope?: WorkbenchScope } = {}): ReactElement {
  const client = useQueryClient();
  const local = props.scope === "local-export";
  const queryKey = local ? workbenchKeys.localExports : workbenchKeys.exports;
  const query = useQuery({
    queryKey,
    queryFn: local ? api.workbenchLocalExports : api.workbenchExports,
  });
  const [deleting, setDeleting] = useState<WorkbenchExportMetadata | null>(null);
  const [regenerating, setRegenerating] = useState<WorkbenchExportMetadata | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const [profileArtifact, setProfileArtifact] = useState<WorkbenchExportMetadata | null>(null);
  const remove = useMutation({
    mutationFn: local ? api.deleteWorkbenchLocalExport : api.deleteWorkbenchExport,
    onSuccess: () => {
      setDeleting(null);
      toast.success("私有文件已删除");
      void client.invalidateQueries({ queryKey });
    },
    onError: (error) => notifyOperationError(error, "私有文件删除失败，请重试"),
  });
  return (
    <section className="grid min-w-0 gap-3 border-t pt-4" aria-label="私有导出文件">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-base font-medium">私有文件</h2>
        <Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>
          <RefreshCw aria-hidden="true" />
          刷新文件
        </Button>
      </div>
      {query.isPending && <ContentLoading label="正在读取私有文件" />}
      {!query.isPending && !query.data && (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {query.data?.length === 0 && (
        <p className="text-sm text-muted-foreground">暂无私有导出文件</p>
      )}
      {query.data && query.data.length > 0 && (
        <ul className="divide-y" aria-label="私有文件列表">
          {query.data.map((item) => (
            <li key={item.id} className="flex min-w-0 flex-wrap items-center gap-3 py-3">
              <div className="min-w-0 flex-1 text-sm">
                <p className="wrap-anywhere">{item.id}</p>
                <p className="text-muted-foreground wrap-anywhere">
                  {item.kind === "login-profiles" ? "登录资料" : "账号授权"} {item.count} 份；到期{" "}
                  {item.expires_at}
                </p>
              </div>
              {local && item.kind === "accounts" ? (
                <Button
                  variant="outline"
                  disabled={query.isError || query.isFetching || remove.isPending}
                  onClick={() => setProfileArtifact(item)}
                  aria-label={`创建文件登录资料 ${item.id}`}
                >
                  <Save aria-hidden="true" />
                  登录资料
                </Button>
              ) : null}
              {local && item.kind === "accounts" ? (
                <Button
                  variant="outline"
                  disabled={query.isError || query.isFetching || remove.isPending}
                  onClick={() => setRegenerating(item)}
                  aria-label={`重新生成私有文件 ${item.id}`}
                >
                  <RefreshCw aria-hidden="true" />
                  再生授权
                </Button>
              ) : null}
              <Button
                variant="outline"
                onClick={() => setDeleting(item)}
                aria-label={`删除私有文件 ${item.id}`}
              >
                <Trash2 aria-hidden="true" />
                删除
              </Button>
            </li>
          ))}
        </ul>
      )}
      {regenerating ? (
        <WorkbenchRegeneration
          source={{ scope: "local-export", artifact_id: regenerating.id }}
          onClose={() => setRegenerating(null)}
          onCreated={setTask}
        />
      ) : null}
      {task ? <WorkbenchTask task={task} /> : null}
      {profileArtifact ? (
        <WorkbenchArtifactProfile
          artifact={profileArtifact}
          onClose={() => setProfileArtifact(null)}
        />
      ) : null}
      <ConfirmActionDialog
        open={deleting !== null}
        title="删除私有文件"
        description={`删除私有文件 ${deleting?.id ?? ""}？文件包含 ${deleting?.count ?? 0} 份资料；删除后无法恢复此文件。`}
        confirmLabel="删除文件"
        pending={remove.isPending}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        onConfirm={() => {
          if (deleting) remove.mutate(deleting.id);
        }}
      />
    </section>
  );
}
