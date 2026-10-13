import { ChevronLeft, ChevronRight } from "lucide-react";
import { useEffect, useMemo, type ReactElement } from "react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useClientPagination } from "@/hooks/use-client-pagination";
import { useIsMobile } from "@/hooks/use-mobile";
import { detectionStageOptions } from "../constants";
import type { useAnimationTasks } from "../hooks/use-animation-tasks";
import { CustomAnimationRecord } from "./custom-animation-record";

export function CustomAnimationRecords(props: {
  mode: "animation" | "precheck";
  tasks: ReturnType<typeof useAnimationTasks>;
  retryDisabled: boolean;
  onRetry: (model: string) => void;
  showEmpty?: boolean;
}): ReactElement | null {
  const statuses = props.mode === "precheck" ? props.tasks.precheckStatuses : props.tasks.statuses;
  // collectAnimationTasks inserts statuses in task creation order, including tasks without results.
  const ids = useMemo(
    () => [...statuses.keys()].filter((id) => id.startsWith("custom-")).reverse(),
    [statuses],
  );
  const mobile = useIsMobile();
  const pagination = useClientPagination(ids, 3);
  const setPageSize = pagination.setPageSize;
  const setCurrentPage = pagination.setCurrentPage;
  const newestID = ids[0];
  useEffect(() => setPageSize(mobile ? 1 : 3), [mobile, setPageSize]);
  useEffect(() => setCurrentPage(1), [newestID, setCurrentPage]);
  const label = detectionStageOptions[props.mode === "precheck" ? 1 : 0].label;
  if (ids.length === 0) {
    return props.showEmpty ? (
      <p className="rounded-lg bg-muted/10 px-4 py-8 text-center text-sm text-muted-foreground">
        暂无{label}记录
      </p>
    ) : null;
  }
  return (
    <section aria-label={`${label}记录`} className="flex shrink-0 flex-col gap-3">
      {pagination.totalPages > 1 ? (
        <nav aria-label={`${label}记录分页`} className="flex items-center justify-end gap-1">
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="上一页记录"
                  disabled={pagination.currentPage === 1}
                  onClick={() => setCurrentPage(pagination.currentPage - 1)}
                />
              }
            >
              <ChevronLeft aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>上一页记录</TooltipContent>
          </Tooltip>
          <span className="text-xs tabular-nums" aria-live="polite">
            {pagination.currentPage}/{pagination.totalPages}
          </span>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="下一页记录"
                  disabled={pagination.currentPage === pagination.totalPages}
                  onClick={() => setCurrentPage(pagination.currentPage + 1)}
                />
              }
            >
              <ChevronRight aria-hidden="true" />
            </TooltipTrigger>
            <TooltipContent>下一页记录</TooltipContent>
          </Tooltip>
        </nav>
      ) : null}
      <div className="grid shrink-0 grid-cols-1 items-start gap-3 md:grid-cols-3">
        {pagination.visibleItems.map((id) => (
          <CustomAnimationRecord
            key={id}
            id={id}
            tasks={props.tasks}
            retryDisabled={props.retryDisabled}
            onRetry={props.onRetry}
          />
        ))}
      </div>
    </section>
  );
}
