import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Star, Trash2, Plus } from "lucide-react";
import { api } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { notifyOperationError } from "@/lib/operation-feedback";
import { cn } from "@/lib/utils";
import { templateKeys } from "../constants";
import type { WorkbenchTemplate } from "../types";
import { ManualTemplateEditor } from "./manual-template-editor";
import { TemplateEditor } from "./template-editor";
import { TemplateDetails } from "./template-details";

export function TemplateList(): ReactElement {
  const client = useQueryClient();
  const query = useQuery({ queryKey: templateKeys.library, queryFn: api.workbenchTemplates });
  const [editor, setEditor] = useState<{ template?: WorkbenchTemplate } | null>(null);
  const [manual, setManual] = useState<{ template?: WorkbenchTemplate } | null>(null);
  const [deleting, setDeleting] = useState<WorkbenchTemplate | null>(null);
  const select = useMutation({
    mutationFn: (id: string) => api.setWorkbenchTemplate(id, query.data!.revision),
    onSuccess: (result) => {
      client.setQueryData(templateKeys.library, result);
    },
    onError: (error) => notifyOperationError(error, "模板切换失败"),
  });
  const remove = useMutation({
    mutationFn: (item: WorkbenchTemplate) =>
      api.deleteWorkbenchTemplate(item.id, query.data!.revision),
    onSuccess: (result) => {
      client.setQueryData(templateKeys.library, result);
      setDeleting(null);
    },
    onError: (error) => notifyOperationError(error, "模板删除失败"),
  });
  const pending = select.isPending || remove.isPending;
  return (
    <section aria-label="配置模板" className="grid min-w-0 gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="grid gap-1">
          <h2 className="text-sm font-semibold">配置模板</h2>
          <p className="text-xs text-muted-foreground">
            手动填写或从线上账号读取配置，供导入时使用。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            disabled={pending || query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw aria-hidden="true" />
            刷新
          </Button>
          <Button variant="outline" disabled={pending || !query.data} onClick={() => setEditor({})}>
            从线上账号创建
          </Button>
          <Button disabled={pending || !query.data} onClick={() => setManual({})}>
            <Plus aria-hidden="true" />
            创建模板
          </Button>
        </div>
      </div>
      {query.isPending && (
        <div aria-label="正在读取模板" aria-busy="true" className="grid gap-3">
          <Skeleton className="h-36" />
          <Skeleton className="h-36" />
        </div>
      )}
      {!query.data && query.isError && (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {query.data && (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/20 p-3 text-sm">
          <span className="min-w-0 flex-1 wrap-anywhere">
            当前模板：
            {query.data.items.find((item) => item.id === query.data.preferred_id)?.name ||
              "自动匹配"}
          </span>
          <Button
            variant="outline"
            disabled={pending || !query.data.preferred_id}
            onClick={() => select.mutate("")}
          >
            自动匹配
          </Button>
        </div>
      )}
      {query.data?.items.length === 0 && (
        <p className="rounded-lg border border-dashed bg-muted/10 px-4 py-12 text-center text-sm text-muted-foreground">
          还没有模板，可手动创建或从线上账号读取。
        </p>
      )}
      <div aria-label="模板列表" className="grid min-w-0 items-start gap-4 xl:grid-cols-2">
        {query.data?.items.map((item) => (
          <article
            key={item.id}
            aria-label={item.name}
            className={cn(
              "min-w-0 rounded-xl border bg-card p-4",
              item.id === query.data.preferred_id && "border-primary/40",
            )}
          >
            <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                <h3 className="wrap-anywhere font-medium">{item.name}</h3>
                <p className="wrap-anywhere text-xs text-muted-foreground">
                  来源：
                  {item.source_id
                    ? item.source_name || `账号 #${item.source_id}`
                    : "手动创建"} ·{" "}
                  {new Date(item.synced_at).toLocaleString("zh-CN")}
                </p>
              </div>
              {item.id === query.data.preferred_id && (
                <span className="inline-flex items-center gap-1 text-xs text-primary">
                  <Star className="size-3.5" aria-hidden="true" />
                  当前使用
                </span>
              )}
            </div>
            <TemplateDetails template={item} />
            <div className="mt-4 flex flex-wrap gap-2 border-t pt-3">
              <Button
                variant="outline"
                disabled={pending || item.id === query.data.preferred_id}
                onClick={() => select.mutate(item.id)}
              >
                <Star aria-hidden="true" />
                设为当前
              </Button>
              <Button
                variant="outline"
                disabled={pending}
                onClick={() => {
                  if (item.source_id) setEditor({ template: item });
                  else setManual({ template: item });
                }}
              >
                <RefreshCw aria-hidden="true" />
                {item.source_id ? "重新同步" : "编辑配置"}
              </Button>
              <Button
                variant="ghost"
                disabled={pending}
                onClick={() => setDeleting(item)}
                aria-label={`删除模板 ${item.name}`}
              >
                <Trash2 aria-hidden="true" />
                删除
              </Button>
            </div>
          </article>
        ))}
      </div>
      {manual && query.data && (
        <ManualTemplateEditor
          template={manual.template}
          revision={query.data.revision}
          onClose={() => setManual(null)}
        />
      )}
      {editor && query.data && (
        <TemplateEditor
          template={editor.template}
          revision={query.data.revision}
          onClose={() => setEditor(null)}
        />
      )}
      {deleting && (
        <ConfirmActionDialog
          open
          title="删除配置模板"
          description={`确定删除“${deleting.name}”吗？线上账号不会被修改。`}
          confirmLabel="删除模板"
          pending={remove.isPending}
          onOpenChange={(open) => {
            if (!open) setDeleting(null);
          }}
          onConfirm={() => remove.mutate(deleting)}
        />
      )}
    </section>
  );
}
