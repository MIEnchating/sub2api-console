import { useQuery } from "@tanstack/react-query";
import { History, RefreshCw } from "lucide-react";
import { useState, type ReactElement } from "react";
import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { formatDateTime } from "@/lib/date-format";
import { taskStatusLabel, taskStatusVariant } from "@/features/system-info/constants";
import { ModelCheckHistoryResult } from "./model-check-history-result";

export function ModelCheckHistory(): ReactElement {
  const [open, setOpen] = useState(false);
  const [page, setPage] = useState(1);
  const [taskID, setTaskID] = useState<string | null>(null);
  const history = useQuery({
    queryKey: ["model-check-history", page],
    queryFn: ({ signal }) => api.modelCheckHistory(page, signal),
    enabled: open,
    staleTime: 0,
    refetchInterval: open ? 15000 : false,
  });
  return (
    <>
      <Button type="button" variant="outline" onClick={() => setOpen(true)}>
        <History aria-hidden="true" />
        检测历史
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          setOpen(value);
          if (!value) setTaskID(null);
        }}
      >
        <DialogContent
          width="wide"
          height="tall"
          className="grid h-[min(44rem,calc(100svh-2rem))] grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>检测历史</DialogTitle>
          </DialogHeader>
          <DialogBody className="flex min-h-0 flex-col gap-3 overflow-hidden pr-0">
            <div className="flex shrink-0 items-center justify-between gap-2">
              <p className="text-sm text-muted-foreground">
                全部账号的模型检测记录，按创建时间倒序排列。
              </p>
              <Button
                variant="outline"
                disabled={history.isFetching}
                onClick={() => void history.refetch()}
              >
                <RefreshCw aria-hidden="true" />
                刷新历史
              </Button>
            </div>
            {history.isError ? (
              <ContentRetry pending={history.isFetching} onRetry={() => void history.refetch()} />
            ) : null}
            <section
              aria-label="模型检测历史列表"
              className="min-h-0 flex-1 overflow-auto rounded-md border"
            >
              {!history.data && history.isPending ? (
                <ContentLoading label="正在读取检测历史" />
              ) : null}
              {history.data?.items.length === 0 ? (
                <p className="p-6 text-center text-sm text-muted-foreground">暂无模型检测历史</p>
              ) : null}
              {history.data?.items.map((task) => (
                <article
                  key={task.id}
                  className="flex min-w-0 flex-wrap items-center justify-between gap-3 border-b p-3 last:border-b-0"
                >
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <time className="text-sm" dateTime={task.created_at}>
                        {formatDateTime(task.created_at)}
                      </time>
                      <StatusBadge
                        label={taskStatusLabel(task.status)}
                        variant={taskStatusVariant(task.status)}
                      />
                    </div>
                    <p className="text-sm wrap-anywhere">{task.message}</p>
                    <p className="text-xs text-muted-foreground wrap-anywhere">
                      任务 ID：{task.id}
                    </p>
                  </div>
                  <Button variant="outline" onClick={() => setTaskID(task.id)}>
                    查看结果
                  </Button>
                </article>
              ))}
            </section>
            <nav aria-label="检测历史分页" className="flex shrink-0 items-center justify-end gap-3">
              <span className="text-xs text-muted-foreground">第 {page} 页 · 每页 20 条</span>
              <Button
                variant="outline"
                disabled={page === 1 || history.isFetching}
                onClick={() => setPage(page - 1)}
              >
                上一页
              </Button>
              <Button
                variant="outline"
                disabled={!history.data?.has_more || history.isFetching || history.isError}
                onClick={() => setPage(page + 1)}
              >
                下一页
              </Button>
            </nav>
          </DialogBody>
          <ModelCheckHistoryResult taskID={taskID} onClose={() => setTaskID(null)} />
        </DialogContent>
      </Dialog>
    </>
  );
}
