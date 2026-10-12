import { useMemo, useState, type ReactElement } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, ChevronDown, Trash2, History, Search } from "lucide-react";

import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { ContentRetry } from "@/components/content-retry";
import { TaskStartupState } from "@/components/task-startup-state";
import { notifyOperationError } from "@/lib/operation-feedback";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { runKeys, runStatusLabels, runStatusVariants, workbenchKeys } from "../constants";
import type { WorkbenchRun } from "../types";
import { WorkbenchToolbar, WorkbenchEmptyState } from "./workbench-section";
import { RunItems } from "./run-items";

type RunAction = {
  run: WorkbenchRun;
  action: "delete" | "cancel" | "retry" | "enable";
  ids?: string[];
};
const actionLabels = {
  delete: "删除记录",
  cancel: "取消任务",
  retry: "继续 / 重试",
  enable: "启用所选账号",
};
function active(run: WorkbenchRun): boolean {
  return run.status === "running" || run.status === "queued";
}

export function RunList(): ReactElement {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: runKeys.list,
    queryFn: ({ signal }) => api.workbenchRuns(signal),
    refetchInterval: 30_000,
  });
  const activeRuns = useMemo(
    () => (query.data ?? []).filter((run) => run.action === "import" && active(run)),
    [query.data],
  );
  const activeQueries = useQueries({
    queries: activeRuns.map((run) => ({
      queryKey: [...runKeys.list, "detail", run.id, run.revision],
      queryFn: ({ signal }: { signal: AbortSignal }) => api.workbenchRun(run.id, signal),
      refetchInterval: (detail: { state: { data?: WorkbenchRun } }) =>
        detail.state.data && active(detail.state.data) ? 1000 : false,
    })),
  });
  const activeUpdates = useMemo(
    () => new Map(activeRuns.map((run, index) => [run.id, activeQueries[index].data])),
    [activeRuns, activeQueries],
  );
  const [search, setSearch] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const [confirmation, setConfirmation] = useState<RunAction | null>(null);
  const update = useMutation({
    mutationFn: (input: RunAction) =>
      api.workbenchRunAction(input.run.id, input.action, {
        revision: input.run.revision,
        ids: input.ids,
      }),
    onSuccess: () => {
      setConfirmation(null);
      void client.invalidateQueries({ queryKey: runKeys.list });
      void client.invalidateQueries({ queryKey: workbenchKeys.accounts });
    },
    onError: (error) => notifyOperationError(error, "记录操作失败"),
  });
  const runs = useMemo(() => {
    const queryText = search.trim().toLowerCase();
    return (query.data ?? [])
      .filter((run) => run.action === "import")
      .map((run) => {
        const updated = activeUpdates.get(run.id);
        return updated && updated.revision >= run.revision ? updated : run;
      })
      .filter((run) => {
        if (!queryText) return true;
        return run.items.some((item) =>
          (item.email + " " + item.name + " " + (item.account_id || ""))
            .toLowerCase()
            .includes(queryText),
        );
      });
  }, [activeUpdates, query.data, search]);
  const pending = update.isPending;
  return (
    <section aria-label="处理记录" className="grid min-w-0 gap-4">
      <WorkbenchToolbar
        description="查看导入进度，继续处理待确认的账号。"
        actions={
          <Button
            variant="outline"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            <RefreshCw aria-hidden="true" />
            刷新
          </Button>
        }
      >
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <div className="relative min-w-0 flex-1 sm:max-w-md">
            <Search
              aria-hidden="true"
              className="pointer-events-none absolute top-2 left-2.5 size-4 text-muted-foreground"
            />
            <Input
              className="pl-9"
              type="search"
              aria-label="搜索处理记录"
              placeholder="搜索邮箱、名称或站点账号 ID"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>
          {query.data && (
            <span className="text-xs text-muted-foreground">共 {runs.length} 个批次</span>
          )}
        </div>
      </WorkbenchToolbar>
      {query.isPending && (
        <div aria-busy="true" aria-label="正在读取处理记录" className="grid gap-3">
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
      )}
      {query.isError && !query.data && (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {query.data && runs.length === 0 && (
        <WorkbenchEmptyState
          icon={History}
          title={search ? "没有匹配的处理记录" : "暂无处理记录，请先导入账号资料"}
          description={
            search ? "调整搜索词后重试。" : "开始处理后，可在这里查看进度与每个账号的结果。"
          }
        />
      )}
      {runs.map((run, index) => {
        const open = expanded === run.id || (expanded === null && index === 0);
        const expired = new Date(run.expires_at).getTime() <= Date.now();
        return (
          <article key={run.id} className="min-w-0 overflow-hidden rounded-lg border bg-card">
            <div className="flex flex-wrap items-center justify-between gap-2 bg-muted/20 p-3 sm:p-4">
              <Button
                variant="ghost"
                aria-expanded={open}
                aria-controls={`run-${run.id}`}
                onClick={() => setExpanded(open ? "" : run.id)}
              >
                <ChevronDown
                  aria-hidden="true"
                  className={cn("transition-transform", !open && "-rotate-90")}
                />
                账号导入 · {run.items.length} 项
              </Button>
              <Badge variant={runStatusVariants[run.status] ?? "neutral"}>
                {runStatusLabels[run.status] || "状态待确认"}
              </Badge>
              <span className="w-full pl-2 text-xs text-muted-foreground sm:ml-auto sm:w-auto sm:pl-0">
                {new Date(run.created_at).toLocaleString("zh-CN")}
              </span>
            </div>
            {open && (
              <div id={`run-${run.id}`} className="grid min-w-0 gap-4 border-t p-3 sm:p-4">
                {run.status === "queued" && <TaskStartupState message="账号处理任务已排队" />}
                {expired && (
                  <p className="text-sm text-muted-foreground">
                    本批登录资料已到期，请重新导入；处理结果仍可查看。
                  </p>
                )}
                <RunItems
                  run={run}
                  pending={pending || active(run) || expired}
                  onEnable={(item) => setConfirmation({ run, action: "enable", ids: [item.id] })}
                />
                <div className="flex flex-wrap items-center gap-2 border-t pt-3">
                  {active(run) ? (
                    <Button
                      variant="outline"
                      disabled={pending}
                      onClick={() => setConfirmation({ run, action: "cancel" })}
                    >
                      取消任务
                    </Button>
                  ) : (
                    <Button
                      variant="outline"
                      disabled={
                        pending ||
                        expired ||
                        !run.items.some(
                          (item) => item.status !== "completed" && item.status !== "exported",
                        )
                      }
                      onClick={() => setConfirmation({ run, action: "retry" })}
                    >
                      继续 / 重试
                    </Button>
                  )}
                  <Button
                    variant="destructive"
                    disabled={pending}
                    className="ml-auto"
                    onClick={() => setConfirmation({ run, action: "delete" })}
                  >
                    <Trash2 aria-hidden="true" />
                    删除记录
                  </Button>
                </div>
              </div>
            )}
          </article>
        );
      })}
      {confirmation && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open && !update.isPending) setConfirmation(null);
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{actionLabels[confirmation.action]}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <p className="text-sm">
                {confirmation.action === "enable"
                  ? "所选账号的糖果题检测未通过，当前未开启调度。确认后将保留检测结果及模板分组，并手动开启调度。"
                  : `${actionLabels[confirmation.action]}：${confirmation.run.items.length} 项账号，创建于 ${new Date(confirmation.run.created_at).toLocaleString("zh-CN")}。`}
              </p>
              {confirmation.action === "delete" && (
                <p className="mt-2 text-sm text-muted-foreground">
                  将停止本批未完成的任务，清理记录及私有资料；线上账号保留。
                </p>
              )}
            </DialogBody>
            <DialogFooter>
              <Button
                variant="outline"
                disabled={update.isPending}
                onClick={() => setConfirmation(null)}
              >
                返回
              </Button>
              <Button disabled={update.isPending} onClick={() => update.mutate(confirmation)}>
                {actionLabels[confirmation.action]}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
