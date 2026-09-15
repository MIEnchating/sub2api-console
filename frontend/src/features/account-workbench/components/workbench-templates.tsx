import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Pencil, Plus, RefreshCw, Star, Trash2 } from "lucide-react";
import { api, type WorkbenchTemplate } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { WorkbenchTemplatesSkeleton } from "./workbench-page-skeletons";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";
import { WorkbenchTemplateDialog } from "./workbench-template-dialog";

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
      api.setPreferredWorkbenchTemplate(item.id, item.revision, !item.preferred),
    onSuccess: () => {
      toast.success("首选模板已更新");
      void client.invalidateQueries({ queryKey: workbenchKeys.templates });
    },
    onError: (error) => notifyOperationError(error, "首选模板更新失败，请刷新后重试"),
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
        <p className="text-sm text-muted-foreground">
          按套餐或邮箱域名匹配，高优先级模板优先；未设置条件的模板作为默认配置。
        </p>
        <Button onClick={() => setEditor({})}>
          <Plus aria-hidden="true" />
          新增配置模板
        </Button>
      </div>
      {!query.data.length && (
        <p className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
          暂无配置模板，可新增或从已有账号提取配置。
        </p>
      )}
      <div className="grid min-w-0 gap-3 lg:grid-cols-2">
        {query.data.map((item) => (
          <article
            key={item.id}
            className="min-w-0 space-y-3 rounded-lg border bg-card p-4"
            aria-label={`配置模板 ${item.name}`}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0 space-y-1">
                <h2 className="font-medium wrap-anywhere">{item.name}</h2>
                {item.preferred && <Badge variant="secondary">首选模板</Badge>}
              </div>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      aria-label={`${item.preferred ? "取消首选模板" : "设为首选模板"}：${item.name}`}
                      aria-pressed={!!item.preferred}
                      disabled={preference.isPending || remove.isPending}
                      onClick={() => preference.mutate(item)}
                    />
                  }
                >
                  <Star
                    aria-hidden="true"
                    className={item.preferred ? "fill-current" : undefined}
                  />
                </TooltipTrigger>
                <TooltipContent>{item.preferred ? "取消首选模板" : "设为首选模板"}</TooltipContent>
              </Tooltip>
            </div>
            <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
              <dt className="text-muted-foreground">匹配条件</dt>
              <dd className="wrap-anywhere">
                {[item.match.plan_type, item.match.email_domain].filter(Boolean).join("、") ||
                  "默认模板"}
              </dd>
              <dt className="text-muted-foreground">匹配优先级</dt>
              <dd>{item.priority}</dd>
              <dt className="text-muted-foreground">账号配置</dt>
              <dd className="wrap-anywhere">
                并发 {item.config.concurrency}；倍率 {item.config.rate_multiplier}；优先级{" "}
                {item.config.priority}
              </dd>
              <dt className="text-muted-foreground">分组 ID</dt>
              <dd className="wrap-anywhere">{item.config.group_ids.join("、") || "未分组"}</dd>
              {item.source_account_id && (
                <>
                  <dt className="text-muted-foreground">来源账号</dt>
                  <dd className="wrap-anywhere">
                    {item.source_name || "来源账号"}（ID {item.source_account_id}）
                  </dd>
                  <dt className="text-muted-foreground">管理目标</dt>
                  <dd className="wrap-anywhere">{item.target_url}</dd>
                  <dt className="text-muted-foreground">来源读取时间</dt>
                  <dd className="wrap-anywhere">{item.source_synced_at || "尚未读取"}</dd>
                </>
              )}
            </dl>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={preference.isPending || remove.isPending}
                onClick={() => setEditor({ item })}
              >
                <Pencil aria-hidden="true" />
                编辑
              </Button>
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
      {editor && (
        <WorkbenchTemplateDialog
          item={editor.item}
          refreshSource={editor.refreshSource}
          onClose={() => setEditor(null)}
        />
      )}
      <ConfirmActionDialog
        open={!!deleting}
        title="删除账号配置模板"
        confirmLabel="删除模板"
        description={`确定删除「${deleting?.name ?? ""}」？已有账号配置不会自动改变。`}
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
