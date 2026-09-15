import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Pencil, Plus, RefreshCw, Check, Trash2 } from "lucide-react";
import { api, type WorkbenchTemplate } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { WorkbenchTemplatesSkeleton } from "./workbench-page-skeletons";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";
import { WorkbenchTemplateDialog } from "./workbench-template-dialog";
import { WorkbenchSelectedTemplate } from "./workbench-selected-template";

export function WorkbenchTemplates(): ReactElement {
  const client = useQueryClient();
  const query = useQuery({ queryKey: workbenchKeys.templates, queryFn: api.workbenchTemplates });
  const [editor, setEditor] = useState<{
    item?: WorkbenchTemplate;
    refreshSource?: boolean;
  } | null>(null);
  const [deleting, setDeleting] = useState<WorkbenchTemplate | null>(null);
  const preference = useMutation({
    mutationFn: (item: WorkbenchTemplate) =>
      api.setPreferredWorkbenchTemplate(item.id, item.revision, true),
    onSuccess: () => {
      toast.success("当前模板已更新");
      void client.invalidateQueries({ queryKey: workbenchKeys.templates });
    },
    onError: (error) => notifyOperationError(error, "模板选择失败，请刷新后重试"),
  });
  const remove = useMutation({
    mutationFn: (item: WorkbenchTemplate) => api.deleteWorkbenchTemplate(item.id, item.revision),
    onSuccess: () => {
      setDeleting(null);
      toast.success("配置模板已删除");
      void client.invalidateQueries({ queryKey: workbenchKeys.templates });
    },
    onError: (error) => notifyOperationError(error, "配置模板删除失败，请刷新后重试"),
  });
  if (query.isPending) return <WorkbenchTemplatesSkeleton />;
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  return (
    <div className="grid min-w-0 gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-medium">配置模板</h2>
        <Button onClick={() => setEditor({})}>
          <Plus aria-hidden="true" />
          读取线上配置
        </Button>
      </div>
      {!query.data.length && (
        <p className="py-12 text-center text-sm text-muted-foreground">还没有保存的模板</p>
      )}
      <div className="min-w-0 divide-y">
        {query.data.map((item) => (
          <article
            key={item.id}
            className="flex min-w-0 flex-col gap-3 py-4 sm:flex-row sm:items-center"
            aria-label={`配置模板 ${item.name}`}
          >
            <div className="min-w-0 flex-1 space-y-1">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <h3 className="min-w-0 font-medium wrap-anywhere">{item.name}</h3>
                {item.preferred && <Badge variant="secondary">当前使用</Badge>}
              </div>
              <WorkbenchSelectedTemplate template={item} />
              <p className="text-xs text-muted-foreground wrap-anywhere">
                {item.source_account_id
                  ? `${item.source_name || "来源账号"}（ID ${item.source_account_id}）`
                  : "已保存模板"}
              </p>
              {item.source_synced_at && (
                <p className="text-xs text-muted-foreground">
                  来源同步于{" "}
                  <time dateTime={item.source_synced_at}>
                    {new Date(item.source_synced_at).toLocaleString("zh-CN")}
                  </time>
                </p>
              )}
            </div>
            <div className="flex shrink-0 flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={!!item.preferred || preference.isPending || remove.isPending}
                aria-pressed={!!item.preferred}
                onClick={() => preference.mutate(item)}
              >
                <Check aria-hidden="true" />
                {item.preferred ? "当前使用" : "使用"}
              </Button>
              {!item.source_account_id && (
                <Button
                  variant="outline"
                  disabled={preference.isPending || remove.isPending}
                  onClick={() => setEditor({ item })}
                >
                  <Pencil aria-hidden="true" />
                  修改名称
                </Button>
              )}
              {item.source_account_id && (
                <Button
                  variant="outline"
                  disabled={preference.isPending || remove.isPending}
                  onClick={() => setEditor({ item, refreshSource: true })}
                >
                  <RefreshCw aria-hidden="true" />
                  刷新来源
                </Button>
              )}
              <Button
                variant="outline"
                disabled={preference.isPending || remove.isPending}
                onClick={() => setDeleting(item)}
              >
                <Trash2 aria-hidden="true" />
                删除
              </Button>
            </div>
          </article>
        ))}
      </div>
      {editor && <WorkbenchTemplateDialog {...editor} onClose={() => setEditor(null)} />}
      <ConfirmActionDialog
        open={deleting !== null}
        title="删除配置模板"
        description={`删除“${deleting?.name ?? ""}”？线上账号保持不变。`}
        confirmLabel="删除模板"
        pending={remove.isPending}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        onConfirm={() => {
          if (deleting) remove.mutate(deleting);
        }}
      />
    </div>
  );
}
